// Test a new deployment of the contract
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/giuliop/HermesVault-frontend/avm"
	"github.com/giuliop/HermesVault-frontend/config"
)

const (
	// relative path to the encrypted file containing the mnemonic of an Algorand account
	encryptedMenmonicFilePath = "files/mnemonic.encrypted"
	// relative path to the directory for saving files
	filesDir = "files/"
)

// Make one deposit and enough withdrawals to test the contract root management
func main() {
	rootCount := 50 // from deployed contract
	txnsCountToTest := rootCount + 5
	depositor, err := getAccountFromEncryptedFile(encryptedMenmonicFilePath)
	if err != nil {
		log.Fatalf("Error getting account from file: %s", err)
	}
	startBalance, err := getAccountBalance(depositor.Address.String())
	if err != nil {
		log.Fatalf("Error getting account balance: %s", err)
	}

	depositAmount := uint64(2 * rootCount * 1e6)
	depositNote, err := sendDeposit(depositor, depositAmount)
	if err != nil {
		log.Fatalf("Error making deposit: %s", err)
	}
	fmt.Printf("Deposit made at trasactions: %v by %s\n", depositNote.TxnID,
		depositor.Address.String())
	txnsCountToTest--

	err = saveNoteToFile(depositNote)
	if err != nil {
		log.Fatalf("Error saving deposit note to file: %s", err)
	}

	// wait 10 seconds to let the txns database update
	time.Sleep(10 * time.Second)

	accounts := make([]crypto.Account, 0, txnsCountToTest)
	note := depositNote
	for i := 1; i <= txnsCountToTest; i++ {
		recipient := crypto.GenerateAccount()
		accounts = append(accounts, recipient)
		err = saveAccountToFile(recipient)
		if err != nil {
			log.Fatalf("Error saving account to file: %s", err)
		}

		note, err = sendWithdrawal(recipient.Address.String(), 1*1e6, note)
		if err != nil {
			log.Fatalf("Error making withdrawal %d/%d: %s", i, txnsCountToTest, err)
		}
		err = saveNoteToFile(note)
		if err != nil {
			log.Fatalf("Error saving change note to file: %s", err)
		}
		fmt.Printf("Withdrawal %d/%d made at transactions: %v by %s with change of %v\n",
			i, txnsCountToTest, note.TxnID, recipient.Address.String(), note.Amount)

		// let's closeout the account to the depositor
		err = closeoutAccount(recipient, depositor.Address.String())
		if err != nil {
			log.Printf("Error closing out account %s: %v", recipient.Address.String(), err)
		}
		fmt.Printf("Account %s closed out\n", recipient.Address.String())
		time.Sleep(5 * time.Second)
	}

	// withdraw the change to the depositor
	note, err = sendWithdrawal(depositor.Address.String(),
		note.MaxWithdrawalAmount().Microalgos, note)
	if err != nil {
		log.Fatalf("Error making withdrawal to depositor: %s", err)
	}
	fmt.Printf("Withdrawal to depositor made at transactions: %v by %s with change of %v\n",
		note.TxnID, depositor.Address.String(), note.Amount)

	// let's check the balance of the depositor is as expected
	finalBalance, err := getAccountBalance(depositor.Address.String())
	if err != nil {
		log.Fatalf("Error getting account balance: %s", err)
	}

	expectedBalance := int(startBalance) -
		config.DepositMinFeeMultiplier*transaction.MinTxnFee -
		len(accounts)*config.WithdrawalMinFee -
		config.WithdrawalMinFee - // final withdrawal fee to depositor
		len(accounts)*transaction.MinTxnFee // closeout fees

	if int(finalBalance) != expectedBalance {
		log.Printf("Expected balance %d, got %d", expectedBalance, finalBalance)
	} else {
		log.Printf("Depositor account %s has expected balance %d", depositor.Address.String(),
			expectedBalance)
	}
}

// getAccountBalance retrieves the balance of an Algorand account
func getAccountBalance(address string) (uint64, error) {
	account, err := avm.AlgodClient().AccountInformation(address).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get account information: %w", err)
	}
	return account.Amount, nil
}

// closeoutAccount closes out an Algorand account to a specified address
func closeoutAccount(account crypto.Account, closeTo string) error {
	algod := avm.AlgodClient()

	sp, err := algod.SuggestedParams().Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get suggested params: %w", err)
	}

	txn, err := transaction.MakePaymentTxn(
		account.Address.String(), // from
		closeTo,                  // to
		0,                        // amount
		[]byte{},                 // note
		closeTo,                  // closeRemainderTo
		sp,                       // suggested params
	)
	if err != nil {
		return fmt.Errorf("failed to make payment transaction: %w", err)
	}

	_, signedTxn, err := crypto.SignTransaction(account.PrivateKey, txn)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	txnID, err := algod.SendRawTransaction(signedTxn).Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	_, err = transaction.WaitForConfirmation(algod, txnID, config.WaitRounds,
		context.Background())
	return err
}
