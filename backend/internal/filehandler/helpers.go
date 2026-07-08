package filehandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func diskSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func sanitizeFilename(name string) string {
	clean := filepath.Base(strings.TrimSpace(name))
	clean = strings.ReplaceAll(clean, "\n", "")
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "\"", "")
	return clean
}

// assembleFile concatenates all chunk files in order into one final file
// and removes the temp chunk directory on success.
func (h *Handler) assembleFile(fileID, totalChunks int) (string, error) {
	chunkDir := filepath.Join(h.tempDir, strconv.Itoa(fileID))
	destPath := filepath.Join(h.storageDir,
		fmt.Sprintf("%d_%d", fileID, time.Now().UnixMilli()))

	dest, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("cannot create destination file: %w", err)
	}
	defer dest.Close()

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("%06d", i))
		chunk, err := os.Open(chunkPath)
		if err != nil {
			return "", fmt.Errorf("missing chunk %d: %w", i, err)
		}
		_, copyErr := io.Copy(dest, chunk)
		chunk.Close()
		if copyErr != nil {
			return "", fmt.Errorf("copy failed at chunk %d: %w", i, copyErr)
		}
	}

	os.RemoveAll(chunkDir)
	return destPath, nil
}

// allFilesComplete returns true when the number of completed files
// matches the expected_files count in the datasets table.
// Uses the provided context so the query respects cancellation from the caller.
func (h *Handler) allFilesComplete(ctx context.Context, datasetID int) (bool, error) {
	var completed, expected int
	err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*), (SELECT expected_files FROM datasets WHERE id=$1)
		FROM files
		WHERE dataset_id=$1 AND upload_status='complete'`,
		datasetID,
	).Scan(&completed, &expected)
	if err != nil {
		return false, err
	}
	return completed == expected && expected > 0, nil
}

// loadAssembledFiles fetches the full list of completed files for a dataset.
// Called just before firing OnFileAssembled so the pipeline has everything it needs.
func (h *Handler) loadAssembledFiles(ctx context.Context, datasetID int) ([]AssembledFile, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT id, original_name, storage_path, size_bytes, coalesce(mime_type, '')
		   FROM files
		  WHERE dataset_id = $1 AND upload_status = 'complete'
		  ORDER BY id`,
		datasetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []AssembledFile
	for rows.Next() {
		var f AssembledFile
		if err := rows.Scan(&f.FileID, &f.OriginalName, &f.StoragePath, &f.SizeBytes, &f.MimeType); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// CleanupDatasetFiles removes final files and any in-progress chunks after
// the database rows have already been deleted.
func (h *Handler) CleanupDatasetFiles(fileIDs []int, storagePaths []string) {
	for _, path := range storagePaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Printf("WARNING: failed to remove dataset file %s: %v\n", path, err)
		}
	}

	for _, fileID := range fileIDs {
		if fileID <= 0 {
			continue
		}
		chunkDir := filepath.Join(h.tempDir, strconv.Itoa(fileID))
		if err := os.RemoveAll(chunkDir); err != nil {
			fmt.Printf("WARNING: failed to remove chunk dir %s: %v\n", chunkDir, err)
		}
	}
}
