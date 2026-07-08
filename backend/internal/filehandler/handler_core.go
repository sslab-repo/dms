package filehandler

import (
	"database/sql"
	"os"
	"path/filepath"
)

// NewHandler creates a Handler and ensures required directories exist.
func NewHandler(storageDir string, db *sql.DB) *Handler {
	tempDir := filepath.Join(storageDir, "_tmp")
	os.MkdirAll(storageDir, 0755)
	os.MkdirAll(tempDir, 0755)
	return &Handler{
		storageDir: storageDir,
		tempDir:    tempDir,
		db:         db,
	}
}
