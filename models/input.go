package models

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/giuliop/HermesVault-frontend/config"

	"github.com/algorand/go-algorand-sdk/v2/types"
)

// Input represents an input string from the user
type Input string

// stringToMicroAlgo converts an input representing an algo amount to an Amount
func (input Input) ToAmount() (Amount, error) {
	intStr, decStr, hasDecimal := strings.Cut(string(input), ".")
	integer, err := strconv.ParseUint(intStr, 10, 64)
	if err != nil {
		return Amount{}, fmt.Errorf("invalid integer part: %w", err)
	}
	var decimal uint64
	if hasDecimal {
		if len(decStr) > 6 {
			return Amount{}, fmt.Errorf("too many decimal places")
		}
		if len(decStr) < 6 {
			decStr += strings.Repeat("0", 6-len(decStr))
		}
		decimal, err = strconv.ParseUint(decStr, 10, 64)
		if err != nil {
			return Amount{}, fmt.Errorf("invalid decimal part: %w", err)
		}
	}
	microalgos := integer*1_000_000 + decimal
	return NewAmount(microalgos), nil
}

// toAddress converts an input to an Address
func (input Input) ToAddress() (Address, error) {
	address, err := types.DecodeAddress(string(input))
	if err != nil {
		return "", fmt.Errorf("error decoding address: %v", err)
	}
	return Address(address.String()), nil
}

// toNote converts an input to a Note
// Input is expected to be a hex-encoded string of 70 bytes (140 hex characters),
// 8 bytes for the amount, 31 bytes for K, 31 bytes for R
// 31 bytes is given from the RandomNonceByteSize constant in package config
func (input Input) ToNote() (*Note, error) {
	amountByteSize := 8
	nonceByteSize := config.RandomNonceByteSize
	amountAndNonceSize := amountByteSize + nonceByteSize

	if len(input) != 140 {
		return nil, errors.New("invalid secret note length")
	}
	decoded, err := hex.DecodeString(string(input))
	if err != nil {
		return nil, fmt.Errorf("error decoding hex string: %v", err)
	}
	amount := decoded[:amountByteSize]
	var k, r [config.RandomNonceByteSize]byte
	copy(k[:], decoded[amountByteSize:amountAndNonceSize])
	copy(r[:], decoded[amountAndNonceSize:])
	note := NewNote(binary.BigEndian.Uint64(amount), k, r)
	return note, nil
}
