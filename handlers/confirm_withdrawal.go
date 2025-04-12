package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/giuliop/HermesVault-frontend/avm"
	"github.com/giuliop/HermesVault-frontend/db"
	"github.com/giuliop/HermesVault-frontend/models"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
)

func ConfirmWithdrawHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, modalWithdrawalFailed("Bad request"), http.StatusBadRequest)
		return
	}
	amount, errAmount := models.Input(r.FormValue("amount")).ToAmount()
	address, errAddress := models.Input(r.FormValue("address")).ToAddress()
	fromNote, errFromNote := models.Input(r.FormValue("fromNote")).ToNote()
	changeNote, errChangeNote := models.Input(r.FormValue("changeNote")).ToNote()

	errorMsg := ""
	if errAmount != nil {
		log.Printf("Error parsing withdrawal amount: %v", errAmount)
		errorMsg += "Invalid withdrawal amount<br>"
	}
	if errAddress != nil {
		log.Printf("Error parsing withdrawal address: %v", errAddress)
		errorMsg += "Invalid withdrawal address<br>"
	}
	if errFromNote != nil {
		log.Printf("Error parsing withdrawal old note: %v", errFromNote)
		errorMsg += "Invalid deposit secret note<br>"
	}
	if errChangeNote != nil {
		log.Printf("Error parsing withdrawal new note: %v", errChangeNote)
		errorMsg += "Invalid new secret note<br>"
	}
	if errorMsg != "" {
		log.Printf("Invalid withdrawal data: %s", errorMsg)
		http.Error(w, modalWithdrawalFailed(errorMsg), http.StatusUnprocessableEntity)
		return
	}
	var err error
	fromNote.LeafIndex, err = db.GetLeafIndexByCommitment(fromNote.Commitment())
	if err != nil {
		log.Printf("Error getting leaf index by commitment: %v", err)
		http.Error(w, modalWithdrawalFailed("Something went wrong"),
			http.StatusInternalServerError)
		return
	}

	withdrawData := &models.WithdrawalData{
		Amount:     amount,
		Fee:        amount.Fee(),
		Address:    address,
		FromNote:   fromNote,
		ChangeNote: changeNote,
	}

	txns, err := avm.CreateWithdrawalTxns(withdrawData)
	if err != nil {
		log.Printf("Error creating withdrawal transactions: %v", err)
		http.Error(w, modalWithdrawalFailed("Something went wrong"),
			http.StatusInternalServerError)
		return
	}

	withdrawData.ChangeNote.TxnID = crypto.GetTxID(txns[0])
	noteId, err := db.RegisterUnconfirmedNote(withdrawData.ChangeNote)
	if err != nil {
		log.Printf("Error saving unconfirmed withdrawal: %v", err)
		http.Error(w, modalWithdrawalFailed("Something went wrong"),
			http.StatusInternalServerError)
		return
	}

	var leafIndex uint64
	var txnId string
	var confirmationError *avm.TxnConfirmationError
	var saveNoteToDbError error

	// We can delete the unconfirmed note if one of these is true:
	// * txn confirmed by the blockchain and the note saved to the database,
	// * txn rejected by the blockchain
	// * internal error sending the txn
	// If we timeout waiting for confirmation or get confirmation but fail to save to the db,
	// we keep the unconfirmed note, the cleanup process will eventually handle it
	defer func() {
		if (confirmationError == nil && saveNoteToDbError == nil) ||
			confirmationError.Type != avm.ErrWaitTimeout {
			db.DeleteUnconfirmedNote(noteId)
		}
	}()

	leafIndex, txnId, confirmationError = avm.SendWithdrawalToNetwork(txns)
	if confirmationError != nil {
		switch confirmationError.Type {
		case avm.ErrRejected:
			log.Printf("Withdrawal transaction rejected: %v", confirmationError.Error())
			msg := `Your withdrawal was rejected by the network.<br>
					Please check your secret note and try again.`
			http.Error(w, modalWithdrawalFailed(msg), http.StatusUnprocessableEntity)
			return
		case avm.ErrWaitTimeout:
			log.Printf("Withdrawal transaction timed out: %v", confirmationError.Error())
			msg := `Your withdrawal has not been confirmed by the blockchain yet.<br>
					Please wait a few minutes and check your wallet to see if the withdrawal was received.<br>
					If not, please try again.`
			http.Error(w, modalWithdrawalFailed(msg), http.StatusRequestTimeout)
			return
		case avm.ErrInternal:
			log.Printf("Internal error sending withdrawal transaction: %v",
				confirmationError.Error())
			msg := `Something went wrong. Your withdrawal was not processed.<br>
					Please try again.`
			http.Error(w, modalWithdrawalFailed(msg), http.StatusInternalServerError)
			return
		default:
			log.Printf("Unknown error sending withdrawal transaction: %v",
				confirmationError.Error())
			msg := `Something went wrong. Your withdrawal was not processed.<br>
					Please try again.`
			http.Error(w, modalWithdrawalFailed(msg), http.StatusInternalServerError)
			return
		}
	}

	withdrawData.ChangeNote.LeafIndex = int(leafIndex)
	if txnId != withdrawData.ChangeNote.TxnID {
		log.Printf("Withdrawal txnId mismatch: %v != %v", txnId, withdrawData.ChangeNote.TxnID)
	}

	successHtml := `
		<dialog class="modal">
		  <h1>&#9989; Withdrawal successful</h1>
		  <p>
			You can use your new secret note to withdraw any remaining balance in the future.
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

	saveNoteToDbError = db.SaveNote(withdrawData.ChangeNote)
	if saveNoteToDbError != nil {
		log.Printf("Error saving withdrawal to db: %v", saveNoteToDbError)
	}
}

func modalWithdrawalFailed(message string) string {
	return `<dialog class="modal">
			    <h1>&#10060; Withdrawal failed</h1>
				<p>
				` + message + `
				</p>
				<button hx-get="withdraw" onclick="this.parentElement.close()">
				  Close
				</button>
			</dialog>
			<script>
			    document.querySelectorAll('dialog')[0].showModal()
			</script>`
}
