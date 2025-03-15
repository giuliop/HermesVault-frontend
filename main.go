package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/db"
	"github.com/giuliop/HermesVault-frontend/frontend/templates"
	"github.com/giuliop/HermesVault-frontend/handlers"
)

// securityHeadersMiddleware adds security headers to all responses
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Content Security Policy
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; frame-ancestors 'none'")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Referrer Policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions Policy
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		next.ServeHTTP(w, r)
	})
}

func main() {
	// Parse the -dev flag
	dev := flag.Bool("dev", false, "run in development mode")
	flag.Parse()

	defer db.Close()

	// Start periodic cleanup of internal database
	db.CleanupUnconfirmedNotes()
	cleanupCancel := db.StartCleanupRoutine(context.Background(), config.CleanupInterval)
	defer cleanupCancel()

	templates.InitTemplates()

	// Create a new mux to apply middleware
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := templates.Main.Execute(w, nil); err != nil {
			log.Printf("Error executing main template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/deposit", handlers.DepositHandler)
	mux.HandleFunc("/withdraw", handlers.WithdrawHandler)
	mux.HandleFunc("/confirm-deposit", handlers.ConfirmDepositHandler)
	mux.HandleFunc("/confirm-withdraw", handlers.ConfirmWithdrawHandler)
	mux.HandleFunc("/stats", handlers.StatsHandler)

	// Serve static files from the "static" directory
	mux.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.Dir("./frontend/static/"))))

	var server *http.Server

	// Apply security headers middleware to all routes
	secureHandler := securityHeadersMiddleware(mux)

	// Determine the mode and configure the server accordingly
	if *dev {
		// Development mode, we use a self-signed certificate to serve HTTPS
		cert, err := tls.LoadX509KeyPair("dev-ssl-certificates/localhost+4.pem",
			"dev-ssl-certificates/localhost+4-key.pem")
		if err != nil {
			log.Fatalf("Error loading certificates: %v", err)
		}

		// Create a custom HTTPS server
		server = &http.Server{
			Addr: ":" + config.DevelopmentPort,
			Handler: secureHandler,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}

		log.Printf("Server running in development mode on port %s\n",
			config.DevelopmentPort)
		go func() {
			err = server.ListenAndServeTLS("", "")
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("Error starting HTTPS server: %v", err)
			}
		}()
	} else {
		// Production mode, we serve HTTP to a reverse proxy
		server = &http.Server{
			Addr: ":" + config.ProductionPort,
			Handler: secureHandler,
		}

		log.Printf("Server running in production mode on port %s\n",
			config.ProductionPort)
		go func() {
			err := server.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("Error starting HTTP server: %v", err)
			}
		}()
	}

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
