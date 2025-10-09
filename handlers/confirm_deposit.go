package handlers

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"

	"github.com/giuliop/HermesVault-frontend/avm"
	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/db"
	"github.com/giuliop/HermesVault-frontend/memstore"
	"github.com/giuliop/HermesVault-frontend/models"

	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

func ConfirmDepositHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, modalDepositFailed("Bad request"), http.StatusBadRequest)
		return
	}
	amount, errAmount := models.Input(r.FormValue("amount")).ToAmount()
	address, errAddress := models.Input(r.FormValue("address")).ToAddress()
	note, errNote := models.Input(r.FormValue("note")).ToNote()

	errorMsg := ""
	if errAmount != nil {
		log.Printf("Error parsing deposit amount: %v", errAmount)
		errorMsg += "Invalid deposit amount<br>"
	}
	if errAddress != nil {
		log.Printf("Error parsing deposit address: %v", errAddress)
		errorMsg += "Invalid Algorand address<br>"
	}
	if errNote != nil {
		log.Printf("Error parsing deposit note: %v", errNote)
		errorMsg += "Invalid note<br>"
	}
	if errorMsg != "" {
		log.Printf("Invalid deposit data: %s", errorMsg)
		http.Error(w, modalDepositFailed(errorMsg), http.StatusUnprocessableEntity)
		return
	}

	signedTxnBase64 := r.FormValue("signedTxn")
	signedTxnBytes, err := base64.StdEncoding.DecodeString(signedTxnBase64)
	if err != nil {
		log.Printf("Error decoding signed transaction: %v", err)
		http.Error(w, modalDepositFailed("The signed transaction is malformed"),
			http.StatusBadRequest)
		return
	}

	var signedTxn types.SignedTxn
	err = msgpack.Decode(signedTxnBytes, &signedTxn)
	if err != nil {
		log.Printf("Error decoding signed transaction: %v", err)
		http.Error(w, modalDepositFailed("The signed transaction is malformed"),
			http.StatusBadRequest)
		return
	}

	groupId := signedTxn.Txn.Group
	ms := memstore.UserSessions
	depositData, err := ms.RetrieveDeposit(groupId)
	if err != nil {
		log.Printf("Error retrieving deposit data: %v", err)
		http.Error(w, modalDepositFailed("Something went wrong"),
			http.StatusInternalServerError)
		return
	}
	ms.DeleteDeposit(groupId)

	if amount.Microalgos != depositData.Amount.Microalgos || address != depositData.Address ||
		note.Text() != depositData.Note.Text() {
		log.Printf("deposit data does not match. Form submitted:\nAmount: %v\nAddress: "+
			"%v\nNote: <redacted>\n, while memory store had Amount: %v\nAddress: %v\n"+
			"Note: <redacted>\n",
			amount, address, depositData.Amount, depositData.Address)
		http.Error(w, modalDepositFailed("Bad Request"), http.StatusBadRequest)
		return
	}

	noteId, err := db.RegisterUnconfirmedNote(depositData.Note)
	if err != nil {
		log.Printf("Error saving unconfirmed deposit: %v", err)
		http.Error(w, modalDepositFailed("Something went wrong"),
			http.StatusInternalServerError)
		return
	}

	var leafIndex uint64
	var txnId string
	var confirmationError *avm.TxnConfirmationError
	var saveNoteToDbError error

	// We can delete the unconfirmed note from the database if one of these is true:
	// * txn confirmed by the blockchain and note saved to the database
	// * error sending the txn other that timeout waiting for confirmation
	// Otherwise we keep the unconfirmed note, the cleanup process will eventually handle it
	defer func() {
		if (confirmationError == nil && saveNoteToDbError == nil) ||
			confirmationError.Type != avm.ErrWaitTimeout {
			db.DeleteUnconfirmedNote(noteId)
		}
	}()

	leafIndex, txnId, confirmationError = avm.SendDepositToNetwork(depositData.Txns,
		signedTxnBytes)

	if confirmationError != nil {
		switch confirmationError.Type {
		case avm.ErrRejected:
			log.Printf("Deposit transaction rejected: %v", confirmationError.Error())
			msg := `Your deposit transaction was rejected by the network.<br>
					Please try again`
			http.Error(w, modalDepositFailed(msg), http.StatusUnprocessableEntity)
			return
		case avm.ErrOverSpend:
			log.Printf("Deposit transaction overspent: %v", confirmationError.Error())
			var msg string
			var maxSpend uint64
			balance, err := avm.GetNetBalance(string(address))
			if err == nil {
				if balance <= config.DepositMinFeeMultiplier*transaction.MinTxnFee {
					maxSpend = 0
				} else {
					maxSpend = balance -
						config.DepositMinFeeMultiplier*transaction.MinTxnFee
				}
				msg = fmt.Sprintf("The maximum amount you can deposit is %s ALGO",
					models.MicroAlgosToAlgoString(maxSpend))
			} else {
				msg = `You do not have enough funds to cover this deposit`
			}
			http.Error(w, modalDepositFailed(msg), http.StatusUnprocessableEntity)
			return
		case avm.ErrMinimumBalanceRequirement:
			log.Printf("Deposit transaction fails MBR: %v", confirmationError.Error())
			var msg string
			var maxSpend uint64
			balance, err := avm.GetNetBalance(string(address))
			if err == nil {
				if balance <= config.DepositMinFeeMultiplier*transaction.MinTxnFee {
					maxSpend = 0
				} else {
					maxSpend = balance -
						config.DepositMinFeeMultiplier*transaction.MinTxnFee
				}
				msg = fmt.Sprintf("The maximum amount you can deposit is %s ALGO",
					models.MicroAlgosToAlgoString(maxSpend))
			} else {
				msg = `You do not have enough funds to cover the account minimum
					   balance requirement with this deposit`
			}
			http.Error(w, modalDepositFailed(msg), http.StatusUnprocessableEntity)
			return
		case avm.ErrExpired:
			log.Printf("Deposit transaction expired: %v", confirmationError.Error())
			msg := `Too much time has passed and your deposit transaction has expired.
					<br>Please try again`
			http.Error(w, modalDepositFailed(msg), http.StatusRequestTimeout)
			return
		case avm.ErrWaitTimeout:
			log.Printf("Deposit transaction timed out: %v", confirmationError.Error())
			msg := `Your deposit has not been confirmed by the network yet.<br>
					Please wait a few minutes and check your wallet to see if the
					deposit was sent.<br>
					If not, please try again.`
			http.Error(w, modalDepositFailed(msg), http.StatusRequestTimeout)
			return
		case avm.ErrInternal:
			log.Printf("Internal error sending deposit transaction: %v",
				confirmationError.Error())
			msg := `Something went wrong. Your deposit was not processed.<br>
					Please try again.`
			http.Error(w, modalDepositFailed(msg), http.StatusInternalServerError)
			return
		}
	}

	// Log successful deposit
	log.Printf("leaf index: %d, type: DEPOSIT, amount: %s ALGO, address: %s",
		leafIndex, models.MicroAlgosToAlgoString(depositData.Amount.Microalgos),
		string(depositData.Address))

	depositData.Note.LeafIndex = leafIndex
	if txnId != depositData.Note.TxnID {
		log.Printf("Deposit txnId mismatch. %v != %v", txnId, depositData.Note.TxnID)
	}

	successHtml := `
		<dialog class="modal">
		  <h1>&#9989; Deposit successful</h1>
		  <p>
			You can use your new secret note to withdraw your funds in the future.
		  </p>
		  <button hx-get="withdraw" onclick="this.parentElement.close()">
			Close
		  </button>
		</dialog>
		<script>
		  document.querySelectorAll('dialog')[0].showModal()
		</script>
	`
	fmt.Fprint(w, successHtml)

	saveNoteToDbError = db.SaveNote(depositData.Note)
	if saveNoteToDbError != nil {
		log.Printf("Error saving deposit to db: %v", saveNoteToDbError)
	}
}

func modalDepositFailed(message string) string {
	return `<dialog class="modal">
			    <h1>&#10060; Deposit failed</h1>
				<p>
				` + message + `
				</p>
				<button hx-get="deposit" onclick="this.parentElement.close()">
				  Close
				</button>
			</dialog>
			<script>
			    document.querySelectorAll('dialog')[0].showModal()
			</script>`
}
