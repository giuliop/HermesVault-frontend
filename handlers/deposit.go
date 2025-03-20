package handlers

import (
	"log"
	"net/http"

	"github.com/giuliop/HermesVault-frontend/avm"
	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/frontend/templates"
	"github.com/giuliop/HermesVault-frontend/memstore"
	"github.com/giuliop/HermesVault-frontend/models"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
)

func DepositHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Cache-Control", config.CacheControl)

		// Check if this is an HTMX request, if not, render the full page
		if RenderFullPageIfNotHtmx(w, r, "deposit") {
			return
		}

		if err := templates.Deposit.Execute(w, nil); err != nil {
			log.Printf("Error executing deposit template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			log.Printf("Error parsing form: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		amount, errAmount := models.Input(r.FormValue("amount")).ToAmount()
		address, errAddress := models.Input(r.FormValue("address")).ToAddress()
		errorMsg := ""
		if errAmount != nil {
			log.Printf("Error parsing deposit amount: %v", errAmount)
			errorMsg += "Invalid algo amount<br>"
		}
		if errAddress != nil {
			log.Printf("Error parsing deposit address: %v", errAddress)
			errorMsg += "Invalid Algorand address<br>"
		}
		if errorMsg != "" {
			http.Error(w, errorMsg, http.StatusUnprocessableEntity)
			return
		}

		note, err := models.GenerateNote(amount.Microalgos)
		if err != nil {
			log.Printf("Error generating new note: %v", err)
			http.Error(w, "Something went wrong. Please try again",
				http.StatusInternalServerError)
			return
		}

		txns, err := avm.CreateDepositTxns(amount, address, note)
		if err != nil {
			log.Printf("Error creating deposit transactions: %v", err)
			http.Error(w, "Something went wrong. Please try again",
				http.StatusInternalServerError)
			return
		}
		note.TxnID = crypto.GetTxID(txns[0])

		depositData := models.DepositData{
			Amount:         amount,
			Address:        address,
			Note:           note,
			Txns:           txns,
			IndexTxnToSign: config.UserDepositTxnIndex,
		}

		ms := memstore.UserSessions
		_, err = ms.StoreDeposit(&depositData)
		if err != nil {
			log.Printf("Error storing deposit: %v", err)
			http.Error(w, "Something went wrong. Please try again",
				http.StatusInternalServerError)
			return
		}

		if err := templates.ConfirmDeposit.Execute(w, &depositData); err != nil {
			log.Printf("Error executing success template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
