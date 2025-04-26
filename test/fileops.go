package main

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/mnemonic"
	"github.com/giuliop/HermesVault-frontend/models"
	"github.com/giuliop/HermesVault-frontend/test/encrypt"
)

// getAccountFromEncryptedFile retrieves an account mnemonic from a filepath,
// asks for the password to decrypt it, and returns the account
func getAccountFromEncryptedFile(path string) (*crypto.Account, error) {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading mnemonic file: %w", err)
	}

	// ignore empty lines and lines starting with `#` or `//`
	// and grab the first non-empty, non-comment line, stripping whitespace
	var ciphertext string
	for _, line := range strings.Split(string(fileBytes), "\n") {
		line = strings.TrimSpace(line)
		if !(line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//")) {
			ciphertext = line
			break
		}
	}
	if ciphertext == "" {
		return nil, errors.New("no valid mnemonic found in file")
	}

	// decrypt the passphrase
	accountMnemonic, err := encrypt.Decrypt(string(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("error reading passphrase file: %w", err)
	}

	privateKey, err := mnemonic.ToPrivateKey(accountMnemonic)
	if err != nil {
		log.Fatalf("failed to get private key from passphrase: %v", err)
	}
	account, err := crypto.AccountFromPrivateKey(ed25519.PrivateKey(privateKey))
	if err != nil {
		log.Fatalf("failed to create account from private key: %v", err)
	}
	return &account, nil
}

// saveAccountToFile saves an Algorand account mnemonic (unencrypted) to a file
// named after the account address in the default filesDir directory
func saveAccountToFile(account crypto.Account) error {
	filePath := filesDir + account.Address.String()
	mnemonic, err := mnemonic.FromPrivateKey(account.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to get mnemonic from private key: %w", err)
	}
	err = os.WriteFile(filePath, []byte(mnemonic), 0644)
	if err != nil {
		return fmt.Errorf("failed to write mnemonic to file: %w", err)
	}
	return nil
}

// saveNoteToFile saves a deposit note (unencrypted) to a file named after the note's
// transaction ID in the default filesDir directory
func saveNoteToFile(note *models.Note) error {
	filePath := filesDir + note.TxnID
	err := os.WriteFile(filePath, []byte(note.Text()), 0644)
	if err != nil {
		return fmt.Errorf("failed to write note to file: %w", err)
	}
	return nil
}
