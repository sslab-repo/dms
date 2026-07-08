// retag applies deterministic domain tags to all ready datasets
// without re-running the AI pipeline.
// Usage: go run scripts/retag.go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"dataset-platform/backend/config"
	"dataset-platform/backend/internal/profiler"
	"dataset-platform/backend/internal/services"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT id, name, coalesce(description,''), coalesce(tags,'{}'), profile_json
		   FROM datasets
		  WHERE status = 'ready'
		  ORDER BY id`,
	)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, description string
		var existingTags []string
		var profileRaw []byte

		if err := rows.Scan(&id, &name, &description, pq.Array(&existingTags), &profileRaw); err != nil {
			log.Printf("dataset %d: scan error: %v", id, err)
			continue
		}

		var profile *profiler.DatasetProfile
		if len(profileRaw) > 0 {
			profile = &profiler.DatasetProfile{}
			if err := json.Unmarshal(profileRaw, profile); err != nil {
				log.Printf("dataset %d: profile parse error: %v", id, err)
				profile = nil
			}
		}

		derived := services.DeriveDomainTags(name, nil, description, profile, nil)
		merged := services.MergeTags(existingTags, derived)

		if _, err := db.ExecContext(ctx,
			`UPDATE datasets SET tags = $1, updated_at = NOW() WHERE id = $2`,
			pq.Array(merged), id,
		); err != nil {
			log.Printf("dataset %d: update error: %v", id, err)
			continue
		}

		fmt.Printf("dataset %d (%s): %v → %v\n", id, name, existingTags, merged)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}
	fmt.Println("Done.")
}
