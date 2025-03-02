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
	return NewAmount(fee)
}

// Round rounds the Algostring to the nearest algo but keeps the Microalgos value
func (a *Amount) Round() *Amount {
	// Round the algostring to the nearest algo
	wholeAlgos := a.Microalgos / 1_000_000
	remainingMicroAlgos := a.Microalgos % 1_000_000
	if remainingMicroAlgos >= 500_000 {
		wholeAlgos++
	}
	a.Algostring = addThousandSeparators(wholeAlgos)
	return a
}

// CalculateFee calculates the fee for a given amount based on config parameters;
// e.g., the fee is 0.1% of the amount with a minimum of 1000 microalgos
func CalculateFee(amount uint64) uint64 {
	return max(amount/config.WithDrawalFeeDivisor, config.WithdrawalMinimumFee)
}

// MicroAlgosToAlgoString converts microalgos (uint64) to a string representing algos.
func MicroAlgosToAlgoString(microalgos uint64) string {
	wholeAlgos := microalgos / 1_000_000
	remainingMicroAlgos := microalgos % 1_000_000

	wholeAlgosStr := addThousandSeparators(wholeAlgos)
	fracStr := fmt.Sprintf("%06d", remainingMicroAlgos)
	fracStr = strings.TrimRight(fracStr, "0")
	if fracStr == "" {
		return wholeAlgosStr
	}
	return fmt.Sprintf("%s.%s", wholeAlgosStr, fracStr)
}

// NewAmount creates a new Amount from a microalgos value
func NewAmount(microalgos uint64) Amount {
	return Amount{
		Algostring: MicroAlgosToAlgoString(microalgos),
		Microalgos: microalgos,
	}
}

// addThousandSeparators adds commas to a number string every 3 digits
func addThousandSeparators(n uint64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	remainder := len(s) % 3
	var result []byte
	if remainder > 0 {
		result = append(result, s[:remainder]...)
		result = append(result, ',')
	}
	for i := remainder; i < len(s); i += 3 {
		result = append(result, s[i:i+3]...)
		if i+3 < len(s) {
			result = append(result, ',')
		}
	}
	return string(result)
}
