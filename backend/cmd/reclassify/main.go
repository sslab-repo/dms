// reclassify re-profiles all ready datasets and updates dataset_type /
// label_completeness using the current profiler code without re-running AI.
// Usage: go run ./cmd/reclassify              (all datasets)
//        go run ./cmd/reclassify <dataset_id> (one dataset)
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"dataset-platform/backend/internal/filehandler"
	"dataset-platform/backend/internal/profiler"
	"dataset-platform/backend/internal/services"

	_ "github.com/lib/pq"
)

func main() {
	dsURL := os.Getenv("DATABASE_URL")
	if dsURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	target := 0
	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatalf("invalid dataset id %q", os.Args[1])
		}
		target = n
	}

	db, err := sql.Open("postgres", dsURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	query := `SELECT id, name, coalesce(dataset_type,''), label_completeness FROM datasets WHERE status='ready'`
	var args []any
	if target > 0 {
		query += ` AND id=$1`
		args = append(args, target)
	}
	query += ` ORDER BY id`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		log.Fatalf("query: %v", err)
	}

	type dsRow struct{ id int; name, currentType string; currentComp float64 }
	var datasets []dsRow
	for rows.Next() {
		var d dsRow
		if err := rows.Scan(&d.id, &d.name, &d.currentType, &d.currentComp); err != nil {
			log.Fatalf("scan: %v", err)
		}
		datasets = append(datasets, d)
	}
	rows.Close()

	for _, d := range datasets {
		fmt.Printf("dataset %d (%s): type=%s completeness=%.2f\n", d.id, d.name, d.currentType, d.currentComp)

		fileRows, err := db.QueryContext(ctx,
			`SELECT id, original_name, coalesce(mime_type,''), storage_path, size_bytes
			   FROM files WHERE dataset_id=$1 ORDER BY id`, d.id)
		if err != nil {
			log.Printf("  ERROR loading files: %v", err)
			continue
		}
		var files []filehandler.AssembledFile
		for fileRows.Next() {
			var f filehandler.AssembledFile
			fileRows.Scan(&f.FileID, &f.OriginalName, &f.MimeType, &f.StoragePath, &f.SizeBytes)
			files = append(files, f)
		}
		fileRows.Close()

		if len(files) == 0 {
			fmt.Printf("  SKIP: no files\n")
			continue
		}

		profile, err := profiler.ProfileDataset(ctx, files)
		if err != nil {
			log.Printf("  ERROR profiling: %v", err)
			continue
		}

		newType, newComp := services.ReclassifyFromProfile(d.currentType, d.currentComp, profile)

		if newType == d.currentType && newComp == d.currentComp {
			fmt.Printf("  no change\n")
			continue
		}

		profileBytes, _ := json.Marshal(profile)
		if _, err := db.ExecContext(ctx,
			`UPDATE datasets SET dataset_type=$1, label_completeness=$2, profile_json=$3::jsonb, updated_at=NOW() WHERE id=$4`,
			newType, newComp, string(profileBytes), d.id,
		); err != nil {
			log.Printf("  ERROR updating: %v", err)
			continue
		}

		fmt.Printf("  UPDATED: type=%s completeness=%.2f (was %s/%.2f)\n",
			newType, newComp, d.currentType, d.currentComp)
	}

	fmt.Println("Done.")
}
