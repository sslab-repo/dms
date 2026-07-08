package filehandler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"dataset-platform/backend/internal/auth"
)

// RegisterFileRequest is the JSON body for POST /api/files/register.
type RegisterFileRequest struct {
	DatasetID    int    `json:"dataset_id"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
}

// RegisterFileResponse is returned after the files row is pre-created.
type RegisterFileResponse struct {
	FileID  int    `json:"file_id"`
	Message string `json:"message"`
}

// RegisterFile pre-creates a files row so the client has a file_id
// before it starts sending chunks. Called via POST /api/files/register.
func (h *Handler) RegisterFile(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context()) // nil for anonymous

	var req RegisterFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.DatasetID == 0 || req.OriginalName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dataset_id and original_name are required"})
		return
	}

	var status string
	var expected int
	var ownerID sql.NullString
	err := h.db.QueryRowContext(r.Context(),
		`SELECT status, expected_files, owner_id::text FROM datasets WHERE id=$1`, req.DatasetID,
	).Scan(&status, &expected, &ownerID)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "dataset not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if !auth.CanUploadForDataset(claims, ownerID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you do not have permission to upload files for this dataset"})
		return
	}
	if status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "dataset is not accepting new files"})
		return
	}
	if expected <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dataset expected_files must be greater than 0"})
		return
	}
	var existing int
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM files WHERE dataset_id = $1`, req.DatasetID,
	).Scan(&existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if existing >= expected {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "all expected files are already registered"})
		return
	}

	var fileID int
	if err := h.db.QueryRowContext(r.Context(),
		`INSERT INTO files (dataset_id, original_name, mime_type, upload_status)
		 VALUES ($1, $2, $3, 'uploading') RETURNING id`,
		req.DatasetID, req.OriginalName, req.MimeType,
	).Scan(&fileID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, RegisterFileResponse{
		FileID:  fileID,
		Message: "File registered. Begin chunk upload using this file_id.",
	})
}
