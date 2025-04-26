// Package encrypt provides simple symmetric, password-based encryption
// and decryption using NaCl’s SecretBox authenticated cipher.
// It reads passwords securely from the terminal
package encrypt

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/term"
)

const saltSize = 16 // size in bytes for the salt

// deriveKey derives a 32-byte key from a password using scrypt.
func deriveKey(password, salt []byte) (*[32]byte, error) {
	key, err := scrypt.Key(password, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	var keyArray [32]byte
	copy(keyArray[:], key)
	return &keyArray, nil
}

// encryptRaw encrypts the plaintext using NaCl's secretbox.
// It returns raw bytes containing: nonce || ciphertext.
func encryptRaw(plaintext []byte, key *[32]byte) ([]byte, error) {
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}
	encrypted := secretbox.Seal(nonce[:], plaintext, &nonce, key)
	return encrypted, nil
}

// decryptRaw decrypts the raw encrypted data produced by encryptRaw.
func decryptRaw(data []byte, key *[32]byte) ([]byte, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("encrypted data too short")
	}
	var nonce [24]byte
	copy(nonce[:], data[:24])
	decrypted, ok := secretbox.Open(nil, data[24:], &nonce, key)
	if !ok {
		return nil, fmt.Errorf("decryption error: invalid key or corrupt data")
	}
	return decrypted, nil
}

// decrypt decrypts the ciphertext asking the user for the password
func Decrypt(ciphertext string) (string, error) {
	fmt.Print("Enter password: ")
	passInput, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal("Failed to read password:", err)
	}
	fmt.Println()

	// Decode the full message (salt || nonce || ciphertext).
	fullData, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode input: %v", err)
	}
	if len(fullData) < saltSize {
		return "", fmt.Errorf("invalid input: missing salt")
	}

	// Extract the salt and the actual encrypted data.
	salt := fullData[:saltSize]
	encryptedRaw := fullData[saltSize:]

	// Derive the key using the extracted salt.
	key, err := deriveKey(passInput, salt)
	if err != nil {
		return "", fmt.Errorf("key derivation failed: %v", err)
	}

	plaintext, err := decryptRaw(encryptedRaw, key)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %v", err)
	}
	return string(plaintext), nil
}

// encrypt encrypts the plaintext asking the user for the password
func Encrytp(plaintext string) (string, error) {
	// Generate a random salt.
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("failed to read random data: %v", err)
	}

	fmt.Print("Enter password: ")
	passInput, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("failed to read password: %v", err)
	}
	fmt.Println()

	// Derive a key from the password using the random salt.
	key, err := deriveKey(passInput, salt)
	if err != nil {
		return "", fmt.Errorf("key derivation failed: %v", err)
	}

	encryptedRaw, err := encryptRaw([]byte(plaintext), key)
	if err != nil {
		return "", fmt.Errorf("encryption failed: %v", err)
	}

	// Prepend the salt to the encrypted data.
	finalData := append(salt, encryptedRaw...)
	encoded := base64.StdEncoding.EncodeToString(finalData)
	return encoded, nil
}
