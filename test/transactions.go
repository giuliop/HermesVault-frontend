package main

import (
	"fmt"
	"log"

	"github.com/algorand/go-algorand-sdk/v2/crypto"

	"github.com/giuliop/HermesVault-frontend/avm"
	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/db"
	"github.com/giuliop/HermesVault-frontend/models"
)

// sendDeposit sends a deposit transaction for the given depositor and amount.
// Returns the deposit note and transactions failures.
// It will log but not return errors in these cases:
//  1. The transaction confirmation timed out (and potentially the note failed to be saved
//     in the database as an unconfirmed note)
//  2. The transaction succeeded but the note failed to be saved in the database
func sendDeposit(depositor *crypto.Account, amountMicroAlgo uint64,
) (note *models.Note, err error) {
	amount := models.NewAmount(amountMicroAlgo)
	address := models.Address(depositor.Address.String())
	note, err = models.GenerateNote(amountMicroAlgo)
	if err != nil {
		return nil, err
	}
	fmt.Printf("generated deposit note: %s\n", note.Text())

	txns, err := avm.CreateDepositTxns(amount, address, note)
	if err != nil {
		return nil, err
	}
	txnToSign := txns[config.UserDepositTxnIndex]
	_, signedTxn, err := crypto.SignTransaction(depositor.PrivateKey, txnToSign)
	if err != nil {
		return nil, err
	}

	var confirmationError *avm.TxnConfirmationError
	note.LeafIndex, note.TxnID, confirmationError = avm.SendDepositToNetwork(txns, signedTxn)
	switch {
	case confirmationError == nil:
		if dbErr := db.SaveNote(note); dbErr != nil {
			log.Printf("failed to save deposit note in db: %v", dbErr)
		}

	case confirmationError.Type == avm.ErrWaitTimeout:
		log.Printf("deposit %s confirmation timed out: %v", note.TxnID, confirmationError)
		if _, dbErr := db.RegisterUnconfirmedNote(note); dbErr != nil {
			log.Printf("failed to register deposit unconfirmed note: %v", dbErr)
		}

	default:
		return nil, confirmationError
	}

	return note, nil
}

// sendWithdrawal sends a withdrawal transaction for the given recipient, amount, and
// fromNote using the TSS.
// Returns the change note and transactions failures
// It will log but not return errors in these cases:
//  1. The transaction confirmation timed out (and potentially the note failed to be saved
//     in the database as an unconfirmed note)
//  2. The transaction succeeded but the note failed to be saved in the database
func sendWithdrawal(recipient string, amountMicroAlgo uint64, fromNote *models.Note,
) (changeNote *models.Note, err error) {
	changeNote, err = models.GenerateChangeNote(models.NewAmount(amountMicroAlgo), fromNote)
	if err != nil {
		return nil, err
	}
	w := models.WithdrawalData{
		Amount:     models.NewAmount(amountMicroAlgo),
		Fee:        models.NewAmount(models.CalculateWithdrawalFee(amountMicroAlgo)),
		Address:    models.Address(recipient),
		FromNote:   fromNote,
		ChangeNote: changeNote,
	}
	fmt.Printf("generated change note: %s\n", changeNote.Text())

	txns, err := avm.CreateWithdrawalTxns(&w)
	if err != nil {
		return nil, err
	}

	var confirmationError *avm.TxnConfirmationError
	changeNote.LeafIndex, changeNote.TxnID, confirmationError =
		avm.SendWithdrawalToNetworkWithTSS(txns)

	switch {
	case confirmationError == nil:
		if dbErr := db.SaveNote(changeNote); dbErr != nil {
			log.Printf("failed to save change note in db: %v", dbErr)
		}

	case confirmationError.Type == avm.ErrWaitTimeout:
		log.Printf("withdrawal %s confirmation timed out: %v", changeNote.TxnID, confirmationError)
		if _, dbErr := db.RegisterUnconfirmedNote(changeNote); dbErr != nil {
			log.Printf("failed to register change unconfirmed note: %v", dbErr)
		}

	default:
		return nil, confirmationError
	}

	return changeNote, nil
}
