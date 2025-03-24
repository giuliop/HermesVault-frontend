package encrypt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/term"
)

var publicKey *[32]byte

const publicKeyRelativePath = "generate-key/public_key.bin"

func init() {
	publicKey = new([32]byte)

	// get the directory of the current file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("No caller information")
	}
	dir := path.Dir(filename)

	file, err := os.Open(filepath.Join(dir, publicKeyRelativePath))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := io.ReadFull(file, publicKey[:]); err != nil {
		panic(err)
	}
}

// Encrypt the provided nullifier
func Encrypt(nullifier []byte) ([]byte, error) {
	// Generate an ephemeral key pair
	ephemeralPublicKey, ephemeralPrivateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// Generate a random nonce (24 bytes)
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}

	// Encrypt the nullifier.
	ciphertext := box.Seal(nil, nullifier, &nonce, publicKey, ephemeralPrivateKey)

	// Prepend the ephemeral public key and nonce to the ciphertext.
	encryptedNullifier := make([]byte, 0, 32+24+len(ciphertext))
	encryptedNullifier = append(encryptedNullifier, ephemeralPublicKey[:]...)
	encryptedNullifier = append(encryptedNullifier, nonce[:]...)
	encryptedNullifier = append(encryptedNullifier, ciphertext...)

	return encryptedNullifier, nil
}

// Decrypt the provided encrypted nullifier
// It prompts the user to enter the secret seed (private key) via a hidden console input,
func Decrypt(ciphertext []byte) ([]byte, error) {
	// Ensure ciphertext is long enough to include the ephemeral public key and nonce.
	if len(ciphertext) < 32+24 {
		return nil, errors.New("ciphertext too short")
	}

	// Extract the ephemeral public key
	var ephemeralPublicKey [32]byte
	copy(ephemeralPublicKey[:], ciphertext[:32])

	// Extract the nonce.
	var nonce [24]byte
	copy(nonce[:], ciphertext[32:32+24])

	// The remainder is the actual ciphertext
	actualCiphertext := ciphertext[32+24:]

	// Prompt the user for the secret seed (input hidden)
	fmt.Print("Enter your secret seed (hex, input hidden): ")
	seedBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("failed to read seed: %v", err)
	}
	fmt.Println() // Move to next line after input

	seedStr := strings.TrimSpace(string(seedBytes))
	seed, err := hex.DecodeString(seedStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode seed: %v", err)
	}

	if len(seed) != 32 {
		return nil, fmt.Errorf("invalid seed length: expected 32 bytes, got %d", len(seed))
	}

	// The seed is the Curve25519 private key
	var recipientPrivateKey [32]byte
	copy(recipientPrivateKey[:], seed)

	// Attempt decryption
	nullifier, ok := box.Open(nil, actualCiphertext, &nonce, &ephemeralPublicKey, &recipientPrivateKey)
	if !ok {
		return nil, errors.New("decryption failed")
	}

	return nullifier, nil
}
