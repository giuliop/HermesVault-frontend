package avm

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/models"
	"github.com/giuliop/HermesVault-frontend/zkp"
	"github.com/giuliop/HermesVault-frontend/zkp/circuits"

	sdk_models "github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/consensys/gnark/frontend"
)

// CreateDepositTxns create the txn group to make a deposit on chain:
// 1. the app call signed by the deposit verifier with the zk proof
// 2. the deposit transaction to the contract address signed by the user
// 3. the additional app call transactions needed to meet the opcode budget
func CreateDepositTxns(amount models.Amount, address models.Address, note *models.Note,
) ([]types.Transaction, error) {

	assignment := &circuits.DepositCircuit{
		Amount:     amount.Microalgos,
		Commitment: note.Commitment(),
		K:          note.K[:],
		R:          note.R[:],
	}
	zkArgs, err := zkp.ZkArgs(assignment, App.DepositCc)
	if err != nil {
		return nil, fmt.Errorf("failed to get zk args for deposit: %v", err)
	}

	depositMethod, err := App.Schema.Contract.GetMethodByName(config.DepositMethodName)
	if err != nil {
		return nil, fmt.Errorf("failed to get method %s: %v", config.DepositMethodName, err)
	}
	appArgs := [][]byte{depositMethod.GetSelector()}
	appArgs = append(appArgs, zkArgs...)

	addressBytes, err := types.DecodeAddress(string(address))
	if err != nil {
		return nil, fmt.Errorf("failed to decode address: %v", err)
	}
	appArgs = append(appArgs, addressBytes[:])

	algod := algodClient()
	sp, err := algod.SuggestedParams().Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get suggested params: %v", err)
	}
	sp.Fee = 0
	sp.FlatFee = true
	sp.LastRoundValid = sp.FirstRoundValid + config.WaitRounds

	// txn1 is the app call signed by the deposit verifier with the zk proof
	txn1, err := transaction.MakeApplicationNoOpTxWithBoxes(
		App.Id,
		appArgs,
		nil, nil, nil, // foreignAccounts, foreignApps, foreignAssets
		[]types.AppBoxReference{
			{AppID: App.Id, Name: []byte("subtree")},
			{AppID: App.Id, Name: []byte("subtree")},
			{AppID: App.Id, Name: []byte("roots")},
			{AppID: App.Id, Name: []byte("roots")},
		},
		sp,
		App.DepositVerifier.Address, // sender
		nil,                         // note
		types.Digest{},              // group
		[32]byte{},                  // lease
		types.ZeroAddress,           // RekeyTo
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make application call txn: %v", err)
	}

	// txn2 is the deposit transaction to the contract address signed by the user
	txn2, err := transaction.MakePaymentTxn(
		string(address), // from
		crypto.GetApplicationAddress(App.Id).String(), // to
		amount.Microalgos,
		nil,                        // note
		types.ZeroAddress.String(), // closeRemainderTo
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make payment txn: %v", err)
	}
	txn2.Fee = transaction.MinTxnFee * config.DepositMinFeeMultiplier

	// additional transactions needed to meet the opcode budget
	// we make them app calls to count also for smart contract opcode pooling.
	txnNeeded := config.VerifierTopLevelTxnNeeded - 2 // 2 transactions already added
	noopMethod, err := App.Schema.Contract.GetMethodByName(config.NoOpMethodName)
	if err != nil {
		return nil, fmt.Errorf("failed to get method %s: %v", config.NoOpMethodName, err)
	}
	args := [][]byte{noopMethod.GetSelector()}

	txns := []types.Transaction{txn1, txn2}
	for i := 0; i < txnNeeded; i++ {
		txn, err := transaction.MakeApplicationNoOpTx(
			App.Id,
			append(args, []byte{byte(i)}), // args
			nil, nil, nil,                 // foreignAccounts, foreignApps, foreignAssets
			sp,
			App.TSS.Address,   // sender
			nil,               // note
			types.Digest{},    // group
			[32]byte{},        // lease
			types.ZeroAddress, // rekeyTo
		)
		if err != nil {
			return nil, fmt.Errorf("failed to make application call txn: %v", err)
		}
		txns = append(txns, txn)
	}

	groupID, err := crypto.ComputeGroupID(txns)
	if err != nil {
		return nil, fmt.Errorf("failed to compute group id: %v", err)
	}
	for i := range txns {
		txns[i].Group = groupID
	}

	return txns, nil
}

// SendDepositToNetwork sends the deposit transactions to the network.
// It returns the leaf index of the deposit note, the ID of the first group txn, and any error
func SendDepositToNetwork(txns []types.Transaction, userSignedTxn []byte,
) (leafIndex uint64, txnId string, txnConfirmationError *TxnConfirmationError) {
	algod := algodClient()
	signedGroup := []byte{}
	// sign the deposit app call transaction with the deposit verifier
	_, signed1, err := crypto.SignLogicSigAccountTransaction(App.DepositVerifier.Account,
		txns[0])
	if err != nil {
		return 0, "", InternalError("failed to sign app call txn: " + err.Error())
	}
	signedGroup = append(signedGroup, signed1...)
	// the second transaction is the one signed by the user
	signedGroup = append(signedGroup, userSignedTxn...)
	// then sign the noop transactions for the opcode budget with the TSS account
	for i := 2; i < len(txns); i++ {
		_, signed, err := crypto.SignLogicSigAccountTransaction(App.TSS.Account, txns[i])
		if err != nil {
			return 0, "", InternalError("failed to sign app call txn: " + err.Error())
		}
		signedGroup = append(signedGroup, signed...)
	}

	// now send the transactions to the network
	_, err = algod.SendRawTransaction(signedGroup).Do(context.Background())
	if err != nil {
		return 0, "", parseSendTransactionError(err)
	}
	// we wait on te first transaction, the deposit app call, to get the leaf index
	depositAppCallTxnId := crypto.GetTxID(txns[0])
	confirmedTxn, err := transaction.WaitForConfirmation(algod, depositAppCallTxnId,
		config.WaitRounds, context.Background())
	if err != nil {
		return 0, "", parseWaitForConfirmationError(err)
	}
	leafIndex, _, err = getLeafIndexAndRoot(confirmedTxn)
	if err != nil {
		return 0, "", InternalError("failed to get leaf index: " + err.Error())
	}
	return leafIndex, depositAppCallTxnId, nil
}

// CreateWithdrawalTxns creates the txn group to make a withdrawal on chain
func CreateWithdrawalTxns(w *models.WithdrawalData) ([]types.Transaction, error) {
	if w.FromNote.LeafIndex == models.EmptyLeafIndex {
		return nil, fmt.Errorf("empty leaf index")
	}

	root, err := getRoot(w.FromNote.LeafIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get root: %v", err)
	}

	merkleProof, err := createMerkleProof(w.FromNote.LeafValue(), w.FromNote.LeafIndex, root)
	if err != nil {
		return nil, fmt.Errorf("failed to create merkle proof: %v", err)
	}
	var path [config.MerkleTreeLevels + 1]frontend.Variable
	for i, v := range merkleProof {
		path[i] = v
	}

	withdrawalRecipient, err := types.DecodeAddress(string(w.Address))
	if err != nil {
		return nil, fmt.Errorf("failed to decode recipient address: %v", err)
	}

	assignment := &circuits.WithdrawalCircuit{
		Recipient:  withdrawalRecipient[:],
		Withdrawal: w.Amount.Microalgos,
		Fee:        w.Fee.Microalgos,
		Commitment: w.ChangeNote.Commitment(),
		Nullifier:  w.FromNote.Nullifier(),
		Root:       root,
		K:          w.FromNote.K[:],
		R:          w.FromNote.R[:],
		Amount:     w.FromNote.Amount,
		Change:     w.ChangeNote.Amount,
		K2:         w.ChangeNote.K[:],
		R2:         w.ChangeNote.R[:],
		Index:      w.FromNote.LeafIndex,
		Path:       path,
	}
	zkArgs, err := zkp.ZkArgs(assignment, App.WithdrawalCc)
	if err != nil {
		return nil, fmt.Errorf("failed to get zk args for withdrawal: %v", err)
	}

	withdrawalMethod, err := App.Schema.Contract.GetMethodByName(config.WithDrawalMethodName)
	if err != nil {
		return nil, fmt.Errorf("failed to get method %s: %v",
			config.WithDrawalMethodName, err)
	}
	args := [][]byte{withdrawalMethod.GetSelector()}
	args = append(args, zkArgs...)

	foreignAccounts := []string{withdrawalRecipient.String()}
	withdrawalRecipientPosInForeignAccounts := 1
	args = append(args, []byte{byte(withdrawalRecipientPosInForeignAccounts)})

	// the fee recipient is the TSS account which will pay the fees
	feeRecipient := App.TSS.Address
	foreignAccounts = append(foreignAccounts, feeRecipient.String())
	feeRecipientPosInForeignAccounts := 2
	args = append(args, []byte{byte(feeRecipientPosInForeignAccounts)})

	// TODO: let the user set noChange
	noChange := false
	noChangeAbi, err := abiEncode(noChange, "bool")
	if err != nil {
		return nil, fmt.Errorf("failed to encode noChange: %v", err)
	}

	args = append(args, noChangeAbi)

	algod := algodClient()
	sp, err := algod.SuggestedParams().Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get suggested params: %v", err)
	}
	sp.Fee = 0
	sp.FlatFee = true
	sp.LastRoundValid = sp.FirstRoundValid + config.WaitRounds

	// txn1 is the app call signed by the withdrawal verifier with the zk proof
	txn1, err := transaction.MakeApplicationNoOpTxWithBoxes(
		App.Id,
		args,
		foreignAccounts,
		nil, nil, // foreignApps, foreignAssets
		[]types.AppBoxReference{
			{AppID: App.Id, Name: w.FromNote.Nullifier()},
			{AppID: App.Id, Name: []byte("subtree")},
			{AppID: App.Id, Name: []byte("roots")},
			{AppID: App.Id, Name: []byte("roots")},
		},
		sp,
		App.WithdrawalVerifier.Address, // sender
		nil,                            // note
		types.Digest{},                 // group
		[32]byte{},                     // lease
		types.ZeroAddress,              // RekeyTo
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make application call txn: %v", err)
	}

	// now we add noop transactions signed by the feeRecipient,
	// the first to pay the fees and the others to meet the opcode budget
	noopMethod, err := App.Schema.Contract.GetMethodByName(config.NoOpMethodName)
	if err != nil {
		return nil, fmt.Errorf("failed to get method %s: %v", config.NoOpMethodName, err)
	}

	txns := []types.Transaction{txn1}
	txnNeeded := config.VerifierTopLevelTxnNeeded - 1 // 1 transaction already added

	for i := range txnNeeded {
		args := [][]byte{noopMethod.GetSelector()}
		txn, err := transaction.MakeApplicationNoOpTx(
			App.Id,
			append(args, []byte{byte(i)}),
			nil, nil, nil, // foreign accounts, foreignApps, foreignAssets
			sp,
			feeRecipient,      // sender
			nil,               // note
			types.Digest{},    // group
			[32]byte{},        // lease
			types.ZeroAddress, // RekeyTo
		)
		if err != nil {
			return nil, fmt.Errorf("failed to make application call txn: %v", err)
		}
		txns = append(txns, txn)
	}
	// set the fee for the first noop transaction
	txns[1].Fee = transaction.MinTxnFee * config.WithdrawalMinFeeMultiplier

	groupID, err := crypto.ComputeGroupID(txns)
	if err != nil {
		return nil, fmt.Errorf("failed to compute group id: %v ", err)
	}
	for i := range txns {
		txns[i].Group = groupID
	}

	return txns, nil
}

// SendWithdrawalToNetworkWithTSS sends the withdrawal txns to the network signed by the TSS.
// It returns the leaf index of the change note, the ID of the first group txn, and any error
func SendWithdrawalToNetworkWithTSS(txns []types.Transaction,
) (leafIndex uint64, txnId string, txnConfirmationError *TxnConfirmationError) {

	algod := algodClient()
	// sign the withdrawal app call transaction with the withdrawal verifier
	signedGroup := []byte{}
	_, signed1, err := crypto.SignLogicSigAccountTransaction(App.WithdrawalVerifier.Account,
		txns[0])
	if err != nil {
		return 0, "", InternalError("failed to sign app call txn: " + err.Error())
	}
	signedGroup = append(signedGroup, signed1...)

	// sign the rest with the TSS
	for i := 1; i < len(txns); i++ {
		_, signed, err := crypto.SignLogicSigAccountTransaction(App.TSS.Account, txns[i])
		if err != nil {
			return 0, "", InternalError("failed to sign app call txn: " + err.Error())
		}
		signedGroup = append(signedGroup, signed...)
	}

	// now send the transactions to the network
	_, err = algod.SendRawTransaction(signedGroup).Do(context.Background())
	if err != nil {
		return 0, "", parseSendTransactionError(err)
	}

	// we wait on te first transaction, the deposit app call, to get the leaf index
	withdrawalAppCallTxnId := crypto.GetTxID(txns[0])
	confirmedTxn, err := transaction.WaitForConfirmation(algod, withdrawalAppCallTxnId,
		config.WaitRounds, context.Background())
	if err != nil {
		return 0, "", parseWaitForConfirmationError(err)
	}
	leafIndex, _, err = getLeafIndexAndRoot(confirmedTxn)
	if err != nil {
		return 0, "", InternalError("failed to get leaf index: " + err.Error())
	}
	return leafIndex, withdrawalAppCallTxnId, nil
}

// getLeafIndexAndRoot extracts the leaf index from the deposit transaction result
func getLeafIndexAndRoot(txn sdk_models.PendingTransactionInfoResponse,
) (leafIndex uint64, root [32]byte, err error) {
	if len(txn.Logs) == 0 {
		return 0, root, fmt.Errorf("no logs in transaction")
	}
	abiBytes := txn.Logs[len(txn.Logs)-1]
	if len(abiBytes) != 4+8+32 {
		return 0, root, fmt.Errorf("invalid log length: expected 12 bytes, got %d", len(abiBytes))
	}
	leafIndex = binary.BigEndian.Uint64(abiBytes[4:12])
	rootBytes := abiBytes[12:]
	copy(root[:], rootBytes)
	log.Printf("leaf index: %d, root: %x\n", leafIndex, root)

	return leafIndex, root, nil
}
