package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"dataset-platform/backend/config"
	"dataset-platform/backend/internal/auth"

	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()

	username := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_USERNAME")))
	password := os.Getenv("ADMIN_PASSWORD")
	displayName := strings.TrimSpace(os.Getenv("ADMIN_DISPLAY_NAME"))
	if displayName == "" {
		displayName = username
	}
	if username == "" || password == "" {
		log.Fatal("ADMIN_USERNAME and ADMIN_PASSWORD are required")
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	var id string
	err = db.QueryRowContext(ctx,
		`INSERT INTO users (username, display_name, password_hash, role)
		 VALUES ($1, $2, $3, 'admin')
		 ON CONFLICT (username)
		 DO UPDATE SET
		     display_name = EXCLUDED.display_name,
		     password_hash = EXCLUDED.password_hash,
		     role = 'admin',
		     updated_at = NOW()
		 RETURNING id::text`,
		username, displayName, passwordHash,
	).Scan(&id)
	if err != nil {
		log.Fatalf("seed admin: %v", err)
	}

	fmt.Printf("Admin user %q ready (id=%s)\n", username, id)
}
