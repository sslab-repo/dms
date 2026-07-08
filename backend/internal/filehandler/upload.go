package filehandler

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"dataset-platform/backend/internal/auth"
)

// ChunkUploadResponse is returned after each chunk POST.
type ChunkUploadResponse struct {
	FileID      int    `json:"file_id"`
	ChunkIndex  int    `json:"chunk_index"`
	TotalChunks int    `json:"total_chunks"`
	Done        bool   `json:"done"`     // true when this file is fully assembled
	AllDone     bool   `json:"all_done"` // true when ALL files in the dataset are assembled
	Error       string `json:"error,omitempty"`
}

// HandleChunkUpload processes a single chunk.
// Attached to POST /api/files/chunk in the router.
//
// Form fields expected:
//
//	file_id      — from RegisterFile
//	chunk_index  — 0-based
//	total_chunks — total number of chunks for this file
//	chunk        — binary data (file field)
func (h *Handler) HandleChunkUpload(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context()) // nil for anonymous

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, ChunkUploadResponse{Error: "invalid multipart form"})
		return
	}

	fileID, err := strconv.Atoi(r.FormValue("file_id"))
	if err != nil || fileID == 0 {
		writeJSON(w, http.StatusBadRequest, ChunkUploadResponse{Error: "invalid file_id"})
		return
	}
	chunkIndex, err := strconv.Atoi(r.FormValue("chunk_index"))
	if err != nil || chunkIndex < 0 {
		writeJSON(w, http.StatusBadRequest, ChunkUploadResponse{Error: "invalid chunk_index"})
		return
	}
	totalChunks, err := strconv.Atoi(r.FormValue("total_chunks"))
	if err != nil || totalChunks < 1 {
		writeJSON(w, http.StatusBadRequest, ChunkUploadResponse{Error: "invalid total_chunks"})
		return
	}
	if chunkIndex >= totalChunks {
		writeJSON(w, http.StatusBadRequest, ChunkUploadResponse{Error: "chunk_index exceeds total_chunks"})
		return
	}

	var datasetID int
	var uploadStatus string
	var ownerID sql.NullString
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT f.dataset_id, f.upload_status, d.owner_id::text
		   FROM files f
		   JOIN datasets d ON d.id = f.dataset_id
		  WHERE f.id = $1`, fileID,
	).Scan(&datasetID, &uploadStatus, &ownerID); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, ChunkUploadResponse{Error: "file_id not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "internal server error"})
		return
	}
	if !auth.CanUploadForDataset(claims, ownerID) {
		writeJSON(w, http.StatusForbidden, ChunkUploadResponse{Error: "you do not have permission to upload chunks for this file"})
		return
	}
	if uploadStatus == "complete" {
		allDone, _ := h.allFilesComplete(r.Context(), datasetID)
		writeJSON(w, http.StatusOK, ChunkUploadResponse{
			FileID:      fileID,
			ChunkIndex:  chunkIndex,
			TotalChunks: totalChunks,
			Done:        true,
			AllDone:     allDone,
		})
		return
	}
	if uploadStatus == "assembling" {
		writeJSON(w, http.StatusConflict, ChunkUploadResponse{Error: "file assembly already in progress"})
		return
	}
	if uploadStatus != "uploading" {
		writeJSON(w, http.StatusConflict, ChunkUploadResponse{Error: "file is not accepting chunks"})
		return
	}

	chunkFile, _, err := r.FormFile("chunk")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ChunkUploadResponse{Error: "missing chunk field"})
		return
	}
	defer chunkFile.Close()

	chunkDir := filepath.Join(h.tempDir, strconv.Itoa(fileID))
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "cannot create chunk dir"})
		return
	}
	chunkPath := filepath.Join(chunkDir, fmt.Sprintf("%06d", chunkIndex))
	dst, err := os.Create(chunkPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "cannot create chunk file"})
		return
	}
	if _, err := io.Copy(dst, chunkFile); err != nil {
		dst.Close()
		writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "internal server error"})
		return
	}
	dst.Close()

	if chunkIndex < totalChunks-1 {
		writeJSON(w, http.StatusOK, ChunkUploadResponse{
			FileID:      fileID,
			ChunkIndex:  chunkIndex,
			TotalChunks: totalChunks,
			Done:        false,
		})
		return
	}

	res, err := h.db.ExecContext(r.Context(),
		`UPDATE files
		    SET upload_status = 'assembling'
		  WHERE id = $1 AND upload_status = 'uploading'`,
		fileID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "internal server error"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var currentStatus string
		if err := h.db.QueryRowContext(r.Context(),
			`SELECT upload_status FROM files WHERE id = $1`,
			fileID,
		).Scan(&currentStatus); err != nil {
			writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "internal server error"})
			return
		}
		if currentStatus == "complete" {
			allDone, _ := h.allFilesComplete(r.Context(), datasetID)
			writeJSON(w, http.StatusOK, ChunkUploadResponse{
				FileID:      fileID,
				ChunkIndex:  chunkIndex,
				TotalChunks: totalChunks,
				Done:        true,
				AllDone:     allDone,
			})
			return
		}
		if currentStatus == "assembling" {
			writeJSON(w, http.StatusConflict, ChunkUploadResponse{Error: "file assembly already in progress"})
			return
		}
		writeJSON(w, http.StatusConflict, ChunkUploadResponse{Error: "file is not accepting chunks"})
		return
	}

	storagePath, err := h.assembleFile(fileID, totalChunks)
	if err != nil {
		if _, updateErr := h.db.ExecContext(r.Context(),
			`UPDATE files SET upload_status = 'error' WHERE id = $1`,
			fileID,
		); updateErr != nil {
			fmt.Printf("WARNING: failed to mark file_id=%d as error after assembly failure: %v\n", fileID, updateErr)
		}
		writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "assembly failed"})
		return
	}
	size := diskSize(storagePath)

	if err := h.db.QueryRowContext(r.Context(),
		`UPDATE files
		    SET storage_path  = $1,
		        size_bytes    = $2,
		        upload_status = 'complete'
		  WHERE id = $3
		  RETURNING dataset_id`,
		storagePath, size, fileID,
	).Scan(&datasetID); err != nil {
		fmt.Printf("WARNING: files row update failed for file_id=%d: %v\n", fileID, err)
		if _, updateErr := h.db.ExecContext(r.Context(),
			`UPDATE files SET upload_status = 'error' WHERE id = $1`,
			fileID,
		); updateErr != nil {
			fmt.Printf("WARNING: failed to mark file_id=%d as error after row update failure: %v\n", fileID, updateErr)
		}
		writeJSON(w, http.StatusInternalServerError, ChunkUploadResponse{Error: "file row update failed"})
		return
	}

	allDone, err := h.allFilesComplete(r.Context(), datasetID)
	if err != nil {
		fmt.Printf("WARNING: allFilesComplete check failed for dataset %d: %v\n", datasetID, err)
	}

	fmt.Printf("[FileHandler] Dataset %d: file %d complete, allDone=%v, pending files check\n", datasetID, fileID, allDone)

	if allDone && h.OnFileAssembled != nil {
		assembledFiles, err := h.loadAssembledFiles(r.Context(), datasetID)
		if err != nil {
			fmt.Printf("WARNING: could not load assembled files for dataset %d: %v\n", datasetID, err)
		} else {
			fmt.Printf("[FileHandler] Dataset %d: ALL files complete (%d files), firing OnFileAssembled\n", datasetID, len(assembledFiles))
			go h.OnFileAssembled(datasetID, assembledFiles)
		}
	} else {
		fmt.Printf("[FileHandler] Dataset %d: NOT firing callback (allDone=%v, OnFileAssembled=%v)\n", datasetID, allDone, h.OnFileAssembled != nil)
	}

	writeJSON(w, http.StatusOK, ChunkUploadResponse{
		FileID:      fileID,
		ChunkIndex:  chunkIndex,
		TotalChunks: totalChunks,
		Done:        true,
		AllDone:     allDone,
	})
}
