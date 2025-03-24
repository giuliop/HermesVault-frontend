package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"log"

	"github.com/giuliop/HermesVault-frontend/db/encrypt"
)

// This is an interactive test that encrypts a test nullifier and then attempts to decrypt it.
// It will prompt the user to enter the secret seed (hex) from generate_key.go.
// Make sure that the public key file exists.
func main() {
	fmt.Println("=== Interactive Encryption/Decryption Test ===")
	fmt.Println("This test will encrypt a test nullifier and then attempt to decrypt it.")
	fmt.Println("When prompted, please input your secret seed (hex) from generate_key.go.")
	fmt.Println("------------------------------------------------------")

	// Create a random test nullifier of exactly 32 bytes.
	nullifier := make([]byte, 32)
	if _, err := rand.Read(nullifier); err != nil {
		log.Fatalf("Failed to generate random nullifier: %v", err)
	}
	if len(nullifier) != 32 {
		log.Fatalf("Test nullifier must be 32 bytes, got %d", len(nullifier))
	}

	// Encrypt the nullifier using the public key loaded from the file.
	ciphertext, err := encrypt.Encrypt(nullifier)
	if err != nil {
		log.Fatalf("Encryption failed: %v", err)
	}

	fmt.Printf("Encryption successful.\nCiphertext (hex): %x\n", ciphertext)
	fmt.Println("------------------------------------------------------")
	fmt.Println("Now the decryption function will be called.")

	// Call Decrypt, which will prompt you for the secret seed.
	decrypted, err := encrypt.Decrypt(ciphertext)
	if err != nil {
		log.Fatalf("Decryption failed: %v", err)
	}

	if !bytes.Equal(nullifier, decrypted) {
		log.Fatalf("Decrypted nullifier does not match original.\nGot:  %x\nWant: %x",
			decrypted, nullifier)
	}

	fmt.Println("Decryption successful. The decrypted nullifier matches the original.")
}
