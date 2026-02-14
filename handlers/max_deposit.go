package handlers

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	"github.com/giuliop/HermesVault-frontend/models"
)

func MaxDepositHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	addressInput := r.URL.Query().Get("address")
	if addressInput == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}

	address, err := models.Input(addressInput).ToAddress()
	if err != nil {
		http.Error(w, "invalid address", http.StatusUnprocessableEntity)
		return
	}

	maxAmount, err := maxDepositAmount(address)
	if err != nil {
		log.Printf("Error computing max deposit amount for %s: %v", address, err)
		http.Error(w, "failed to compute max deposit amount", http.StatusInternalServerError)
		return
	}
	log.Printf("Max deposit amount for %s: %d microAlgos", address, maxAmount)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	maxAmountInput := html.EscapeString(microAlgosToInputAmount(maxAmount))
	_, err = fmt.Fprintf(w, `<input type="number" id="depositAmount" name="amount" data-wallet-amount placeholder="algo to deposit" step="0.000001" min="1" required value="%s">`, maxAmountInput)
	if err != nil {
		log.Printf("Error rendering max deposit input: %v", err)
	}
}

func microAlgosToInputAmount(microalgos uint64) string {
	whole := microalgos / 1_000_000
	fraction := microalgos % 1_000_000
	if fraction == 0 {
		return fmt.Sprintf("%d", whole)
	}
	fractionStr := fmt.Sprintf("%06d", fraction)
	fractionStr = strings.TrimRight(fractionStr, "0")
	return fmt.Sprintf("%d.%s", whole, fractionStr)
}
