// This program generates a Curve25519 key pair.
// The public key is saved to a file in the current directory and used by the database
// to encrypt nullifers.
// The private key is displayed to the user and should be stored securely for later use
// when decrypting the nullifiers.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/curve25519"
)

func main() {
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Generate a random 32-byte private key (seed) for Curve25519.
	privateKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, privateKey); err != nil {
		log.Fatalf("Failed to generate random private key: %v", err)
	}

	// Clamp the private key as required by Curve25519.
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	seedHex := hex.EncodeToString(privateKey)

	// Derive the public key using the Curve25519 base point.
	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		log.Fatalf("Failed to derive public key: %v", err)
	}

	// Create the public key filename.
	pubKeyFilename := filepath.Join(currentDir, "public_key.bin")

	// Write the public key to file.
	if err := os.WriteFile(pubKeyFilename, publicKey, 0600); err != nil {
		log.Fatalf("Failed to write public key to file: %v", err)
	}

	// Display the information to the user.
	fmt.Printf("\n==== SEED AND PUBLIC KEY INFORMATION ====\n\n")
	fmt.Printf("Random Seed (KEEP THIS SECRET AND SAFE):\n%s\n\n", seedHex)
	fmt.Printf("Public Key (hex):\n%s\n\n", hex.EncodeToString(publicKey))
	fmt.Printf("Public Key File:\n%s\n\n", pubKeyFilename)
	fmt.Printf("============================================\n\n")
	fmt.Printf("IMPORTANT: Store the seed securely offline. It will NOT be saved to disk.\n")
	fmt.Printf("           The public key has been saved to the file shown above.\n\n")
}
