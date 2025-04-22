// package models contains the data types used in the application and methods on them
package models

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"math"

	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

const (
	EmptyTxnId     = ""
	EmptyLeafIndex = math.MaxUint64
)

type WithdrawalData struct {
	Amount     Amount
	Fee        Amount
	Address    Address
	FromNote   *Note
	ChangeNote *Note
}

type DepositData struct {
	Amount         Amount
	Address        Address
	Note           *Note
	Txns           []types.Transaction // the deposit transaction group
	IndexTxnToSign int                 // index of the transaction the user has to sign
}

func (d *DepositData) TxnsJson() string {
	json := EncodeTxnsToJson(d.Txns)
	return json
}

// EncodeTxns encodes each transactions to msgpack, then to base64 string, then packs them
// into an array and encodes them to JSON string
func EncodeTxnsToJson(txns []types.Transaction) string {
	var base64Txns []string
	for _, txn := range txns {
		msgpackTxn := msgpack.Encode(txn)
		base64Txns = append(base64Txns, base64.StdEncoding.EncodeToString(msgpackTxn))
	}
	jsonData, err := json.Marshal(base64Txns)
	// should not happen
	if err != nil {
		log.Printf("failed to convert to JSON: %v", err)
	}
	return string(jsonData)
}

type StatData struct {
	DepositTotal    Amount
	WithdrawalTotal Amount
	FeeTotal        Amount
	DepositCount    uint64
	NoteCount       uint64
}

func (s *StatData) TVL() *Amount {
	tvl := s.DepositTotal.Microalgos - s.WithdrawalTotal.Microalgos - s.FeeTotal.Microalgos
	tvlAmount := NewAmount(tvl)
	return &tvlAmount
}
