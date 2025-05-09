package handlers

import (
	"log"
	"net/http"

	"github.com/giuliop/HermesVault-frontend/db"
	"github.com/giuliop/HermesVault-frontend/frontend/templates"
)

// Stats represents the statistics data to be displayed on the stats page
type Stats struct {
	DepositCount    uint64
	NoteCount       uint64
	SpentNoteCount  uint64
	DepositTotal    string
	WithdrawalTotal string
	TVL             string
	FeeTotal        string
}

func StatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300") // 300 sec = 5 min

	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if this is an HTMX request, if not, render the full page
	if RenderFullPageIfNotHtmx(w, r, "stats") {
		return
	}

	// Get stats from the database
	statData, err := db.GetStats()
	if err != nil {
		log.Printf("Error retrieving stats: %v", err)
		http.Error(w, "Error retrieving statistics, try again later",
			http.StatusInternalServerError)
		return
	}

	stats := &Stats{
		DepositCount:    statData.DepositCount,
		NoteCount:       statData.NoteCount,
		SpentNoteCount:  statData.NoteCount - statData.DepositCount,
		DepositTotal:    statData.DepositTotal.Round().Algostring,
		WithdrawalTotal: statData.WithdrawalTotal.Round().Algostring,
		TVL:             statData.TVL().Round().Algostring,
		FeeTotal:        statData.FeeTotal.Round().Algostring,
	}

	if err := templates.Stats.Execute(w, stats); err != nil {
		log.Printf("Error executing stats template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
