package main

import (
	"context"
	"database/sql"
	"dataset-platform/backend/config"
	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/api"
	"dataset-platform/backend/internal/filehandler"
	"dataset-platform/backend/internal/search"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

/*

This is the entry point of the entire platform

When the application starts, this file runs first. It loads configuration, connects to the datasbse,
initializes each internal component, wires them together, and starts the HTTP server.

Nothing heavy lives here. Every real piece of logic lives in internal/

main.go is purely the startup sequence

*/

func main() {

	// Loading the configuration from enviornment variables
	cfg := config.Load()

	log.Printf("Starting dataset platform on port %s", cfg.ServerPort)
	log.Printf("Database: %s", maskDSNPassword(cfg.DatabaseURL))
	log.Printf("AI API: %s (model: %s)", cfg.AIBaseURL, cfg.AIModel)
	log.Printf("Storage: %s", cfg.FileStorageDir)
	if cfg.JWTSecret == "dev-secret-change-me" {
		log.Println("WARNING: JWT_SECRET is using the development default; set a long random secret in production")
	}

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	// Verify the connection is acutually live before going further
	// sql.Open is lazy - it does not dial until first query
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Cannot reach PostgreSQL on %s: %v", cfg.DatabaseURL, err)
	}
	log.Println("PostgreSQL connection OK")

	// Tunining the connection pool
	// MaxOpenConns limits how many simultaneous database connections we hold
	// MaxIdleConns keeps a few warm connections ready so we are not paying dial overhead on every request
	// ConnMaxLifetime recycles connections periodically so stale ones do not linger forever
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Ensuring the file storage directory exists
	if err := os.MkdirAll(cfg.FileStorageDir, 0755); err != nil {
		log.Fatalf("Cannot create file storage directory %s: %v", cfg.FileStorageDir, err)
	}

	// Initalize internal components
	// Each component receives only what it needs. None of them import each other directly
	aiClient := ai.NewClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.EmbeddingURL, cfg.EmbeddingModel, cfg.EmbeddingAPIKey, cfg.EmbeddingDimensions, cfg.AIModel)
	fileHandler := filehandler.NewHandler(cfg.FileStorageDir, db)
	searchService := search.NewService(db, aiClient, cfg.SemanticSimilarityThreshold)
	router := api.NewRouter(db, aiClient, fileHandler, searchService, cfg.FileStorageDir, cfg.JWTSecret, time.Duration(cfg.JWTExpiryHours)*time.Hour)

	// Starting the HTTP server
	// We use a custom http.Server so we can set read/write timeouts that protect the server from slow clients
	// WriteTimeout is intentionally generous at 30 mins because downloading huge datasets takes time
	// The file handler streams the response so memory is not an issue but we stil need to give the transfer enough time to complete
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.ServerPort),
		Handler:      router,
		ReadTimeout:  15 * time.Minute,
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// Run the server in a goroutine so the main goroutine can listen for shutdown signals
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on :%s", cfg.ServerPort)
		serverErr <- server.ListenAndServe()
	}()

	// Graceful shutdown
	// Wait for either a fatal server error or an OS signal (Ctrl-C or systemd stop).
	// On signal, give in-flight requests up to 30 seconds to finish before the process exits. This is important for large file uploads that may be mid-transfer.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Fatalf("Server error: %v", err)
	case sig := <-quit:
		log.Printf("Received signal %s — shutting down gracefully", sig)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Graceful shutdown failed: %v", err)
	}
	log.Println("Server stopped cleanly")
}

// maskDSNPassword replaces the password in a postgres DSN with ***
// so it is safe to include in log output.
func maskDSNPassword(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "[unparseable DSN]"
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
	}
	return u.String()
}
