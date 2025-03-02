package models

import (
	"fmt"
	"strings"

	"github.com/giuliop/HermesVault-frontend/config"
)

// Amount represent an algo token amount
type Amount struct {
	Algostring string
	Microalgos uint64
}

// Fee calculates the fee for a withdrawal amount
func (withdrawalAmount *Amount) Fee() Amount {
	fee := CalculateFee(withdrawalAmount.Microalgos)
	return Amount{
		Algostring: MicroAlgosToAlgoString(fee),
		Microalgos: fee,
	}
}

// CalculateFee calculates the fee for a given amount; the fee is 0.1% of the amount with a minimum of 1000 microalgos
func CalculateFee(amount uint64) uint64 {
	fee := amount / config.WithDrawalFeeDivisor
	if fee < config.WithdrawalMinimumFee {
		fee = config.WithdrawalMinimumFee
	}
	return fee
}

// MicroAlgosToAlgoString converts microalgos (uint64) to a string representing algos.
func MicroAlgosToAlgoString(microalgos uint64) string {
	const microAlgosPerAlgo = 1_000_000
	wholeAlgos := microalgos / microAlgosPerAlgo
	remainingMicroAlgos := microalgos % microAlgosPerAlgo
	s := fmt.Sprintf("%d.%06d", wholeAlgos, remainingMicroAlgos)
	s = strings.TrimRight(s, "0")
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// NewAmount creates a new Amount from a microalgos value
func NewAmount(microalgos uint64) Amount {
	return Amount{
		Algostring: MicroAlgosToAlgoString(microalgos),
		Microalgos: microalgos,
	}
}
