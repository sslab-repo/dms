package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"dataset-platform/backend/internal/auth"
	"dataset-platform/backend/internal/filehandler"
	"dataset-platform/backend/internal/services"
)

type DatasetFileHandler struct {
	fileSvc     *services.DatasetFileService
	fileHandler *filehandler.Handler
}

func NewDatasetFileHandler(
	fileSvc *services.DatasetFileService,
	fileHandler *filehandler.Handler,
) *DatasetFileHandler {
	return &DatasetFileHandler{fileSvc: fileSvc, fileHandler: fileHandler}
}

type registerAdditionalFileRequest struct {
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
}

func (h *DatasetFileHandler) HandleDatasetFileRegister(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	datasetID, ok := parseDatasetFileRegisterPath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	claims, ok := auth.ClaimsFromContext(req.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var body registerAdditionalFileRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	result, err := h.fileSvc.RegisterAdditionalFile(req.Context(), services.RegisterAdditionalFileInput{
		DatasetID:    datasetID,
		OriginalName: body.OriginalName,
		MimeType:     body.MimeType,
	}, claims)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *DatasetFileHandler) HandleFileByID(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fileID, ok := parseFilePath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	claims, ok := auth.ClaimsFromContext(req.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	result, err := h.fileSvc.DeleteDatasetFile(req.Context(), fileID, claims)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	h.fileHandler.CleanupDatasetFiles([]int{result.FileID}, []string{result.StoragePath})
	if err := h.fileHandler.TriggerDatasetProcessing(req.Context(), result.DatasetID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseDatasetFileRegisterPath(path string) (int, bool) {
	path = strings.TrimPrefix(path, "/api/datasets/")
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[1] != "files" || parts[2] != "register" {
		return 0, false
	}
	id, err := strconv.Atoi(parts[0])
	return id, err == nil && id > 0
}

func parseFilePath(path string) (int, bool) {
	idText := strings.TrimPrefix(path, "/api/files/")
	if idText == "" || strings.Contains(idText, "/") {
		return 0, false
	}
	id, err := strconv.Atoi(idText)
	return id, err == nil && id > 0
}
