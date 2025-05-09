package config

import (
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/consensys/gnark-crypto/ecc"
)

// app constants
const (
	MerkleTreeLevels    = 32
	Curve               = ecc.BN254
	RandomNonceByteSize = 31

	DepositMinimumAmount = 1_000_000 // microalgo, or 1 algo

	DepositMethodName    = "deposit"
	WithDrawalMethodName = "withdraw"
	NoOpMethodName       = "noop"

	UserDepositTxnIndex = 1 // index of the user pay txn in the deposit txn group (0 based)
)

// transaction fees required
const (
	// # top level transactions needed for logicsig verifier opcode budget
	VerifierTopLevelTxnNeeded = 8

	// fees needed for a deposit transaction group
	DepositMinFeeMultiplier = 56

	// fees needed for a withdrawal transaction group
	WithdrawalMinFeeMultiplier = 60
	// fees withhold by the smart contract
	NullifierMbr = 15_300 // microalgo, or 0.0153 algo
	// min fees for withdrawal to be taken from the deposit note
	WithdrawalMinFee = NullifierMbr + WithdrawalMinFeeMultiplier*transaction.MinTxnFee
)

var Hash = NewMimcF(Curve)
