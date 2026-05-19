package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/db"
	"github.com/giuliop/HermesVault-frontend/frontend/templates"
	"github.com/giuliop/HermesVault-frontend/handlers"
)

func main() {

	defer db.Close()

	// Start periodic cleanup of internal database
	db.CleanupUnconfirmedNotes()
	cleanupCancel := db.StartCleanupRoutine(context.Background(), config.CleanupInterval)
	defer cleanupCancel()

	templates.InitTemplates()

	http.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if err := templates.Main.Execute(w, nil); err != nil {
			log.Printf("Error executing main template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/deposit", handlers.DepositHandler)
	http.HandleFunc("/withdraw", handlers.WithdrawHandler)
	http.HandleFunc("/confirm-deposit", handlers.ConfirmDepositHandler)
	http.HandleFunc("/confirm-withdraw", handlers.ConfirmWithdrawHandler)
	http.HandleFunc("/max-deposit", handlers.MaxDepositHandler)
	http.HandleFunc("/stats", handlers.StatsHandler)

	// Serve static files from the "static" directory
	http.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.Dir("./frontend/static/"))))

	server := &http.Server{
		Addr:              ":" + config.Port,
		Handler:           subscriberHealthGuard(http.DefaultServeMux),
		ReadHeaderTimeout: 60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       300 * time.Second,
	}

	log.Printf("Server running on port %s\n", config.Port)
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting HTTP server: %v", err)
		}
	}()

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	// Block the main goroutine until the server is shut down
	<-quit
	log.Print("\nShutting down server...\n\n")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v\n", err)
	}
}

func subscriberHealthGuard(next http.Handler) http.Handler {
	fatalPath := filepath.Join(filepath.Dir(config.TxnsDbPath), "subscriber.fatal")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(fatalPath); err == nil {
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		} else if err != nil && !os.IsNotExist(err) {
			log.Printf("Error checking subscriber fatal marker: %v", err)
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}

		next.ServeHTTP(w, r)
	})
}
