package models

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"github.com/giuliop/HermesVault-frontend/config"
)

type Note struct {
	Amount    uint64
	K         [config.RandomNonceByteSize]byte
	R         [config.RandomNonceByteSize]byte
	LeafIndex int
	TxnID     string
}

// GenerateNote generates a new note for a given amount
func GenerateNote(amount uint64) (*Note, error) {
	k, errK := generateRandomNonce()
	r, errR := generateRandomNonce()
	if errK != nil || errR != nil {
		return nil, fmt.Errorf("error generating random bytes for k / r: %v / %v",
			errK, errR)
	}
	return &Note{
		Amount: amount,
		K:      k,
		R:      r,
	}, nil
}

func (n *Note) Text() string {
	return fmt.Sprintf("%016x%x%x", n.Amount, n.K, n.R)
}

func (n *Note) Nullifier() []byte {
	k32Byte := append([]byte{0}, n.K[:]...)
	return config.Hash(uint64ToBytes32(n.Amount), k32Byte)
}

func (n *Note) Commitment() []byte {
	return config.Hash(n.LeafValue())
}

func (n *Note) LeafValue() []byte {
	ab := uint64ToBytes32(n.Amount)
	k32Byte := append([]byte{0}, n.K[:]...)
	r32Byte := append([]byte{0}, n.R[:]...)
	h := config.Hash(ab, k32Byte, r32Byte)
	return h
}

func (n *Note) MaxWithdrawalAmount() Amount {
	fee := CalculateWithdrawalFee(n.Amount)
	if n.Amount <= fee {
		return NewAmount(0)
	}
	return NewAmount(n.Amount - fee)
}

// generateDepositNote generates a new deposit note for the change amount after a withdrawal
func GenerateChangeNote(withdrawalAmount Amount, fromNote *Note) (*Note, error) {
	deduction := withdrawalAmount.Microalgos + CalculateWithdrawalFee(withdrawalAmount.Microalgos)
	if deduction < withdrawalAmount.Microalgos {
		return nil, fmt.Errorf("overflow in deduction")
	}
	if fromNote.Amount < deduction {
		return nil, fmt.Errorf("note amount too small")
	}
	change := fromNote.Amount - deduction

	note, err := GenerateNote(change)
	if err != nil {
		return nil, fmt.Errorf("error generating note: %v", err)
	}
	return note, nil
}

// generateRandomNonce generates a cryptographically secure byte array of size
// config.RandomNonceByteSize
func generateRandomNonce() ([config.RandomNonceByteSize]byte, error) {
	var arr [config.RandomNonceByteSize]byte
	_, err := rand.Read(arr[:])
	if err != nil {
		return [config.RandomNonceByteSize]byte{},
			fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return arr, nil
}

// uint64ToBytes32 converts a uint64 to a 32 byte array
func uint64ToBytes32(amount uint64) []byte {
	amountBytes := make([]byte, 32)
	binary.BigEndian.PutUint64(amountBytes[24:], amount)
	return amountBytes
}
