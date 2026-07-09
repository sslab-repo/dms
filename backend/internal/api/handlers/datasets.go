package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"dataset-platform/backend/internal/auth"
	"dataset-platform/backend/internal/filehandler"
	"dataset-platform/backend/internal/mlexport"
	"dataset-platform/backend/internal/services"
)

type DatasetHandler struct {
	datasetSvc  *services.DatasetService
	fileHandler *filehandler.Handler
	exportSvc   *mlexport.Service
}

func NewDatasetHandler(datasetSvc *services.DatasetService, fileHandler *filehandler.Handler, exportSvc *mlexport.Service) *DatasetHandler {
	return &DatasetHandler{datasetSvc: datasetSvc, fileHandler: fileHandler, exportSvc: exportSvc}
}

type CreateDatasetRequest struct {
	Name            string   `json:"name"`
	ResearcherName  string   `json:"researcher_name"`
	UploaderEmail   string   `json:"uploader_email"`
	UserDescription string   `json:"user_description"`
	Tags            []string `json:"tags"`
	TotalFiles      int      `json:"total_files"`
	LabelColumn     string   `json:"label_column"`
}

type CreateDatasetResponse struct {
	DatasetID int    `json:"dataset_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type UpdateDatasetRequest struct {
	Name            string   `json:"name"`
	ResearcherName  string   `json:"researcher_name"`
	UserDescription string   `json:"user_description"`
	Tags            []string `json:"tags"`
	LabelColumn     string   `json:"label_column"`
}

func (h *DatasetHandler) HandleDatasets(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		h.listDatasets(w, req)
	case http.MethodPost:
		h.createDataset(w, req)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *DatasetHandler) HandleDatasetByID(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/datasets/")
	parts := strings.SplitN(path, "/", 2)

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid dataset id", http.StatusBadRequest)
		return
	}

	if len(parts) == 2 && parts[1] == "download" {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := req.URL.Query()
		q.Set("dataset_id", strconv.Itoa(id))
		req.URL.RawQuery = q.Encode()
		h.fileHandler.HandleDownload(w, req)
		return
	}

	if len(parts) == 2 && parts[1] == "export" {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.downloadExport(w, req, id)
		return
	}

	switch req.Method {
	case http.MethodGet:
		h.getDataset(w, req, id)
	case http.MethodPut:
		h.updateDataset(w, req, id)
	case http.MethodDelete:
		h.deleteDataset(w, req, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *DatasetHandler) createDataset(w http.ResponseWriter, req *http.Request) {
	claims, _ := auth.ClaimsFromContext(req.Context()) // nil for anonymous

	var body CreateDatasetRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	ownerID := ""
	if claims != nil {
		ownerID = claims.UserID
	}

	result, err := h.datasetSvc.CreateDataset(req.Context(), services.CreateDatasetInput{
		Name:            body.Name,
		ResearcherName:  body.ResearcherName,
		UploaderEmail:   body.UploaderEmail,
		UserDescription: body.UserDescription,
		Tags:            body.Tags,
		TotalFiles:      body.TotalFiles,
		OwnerID:         ownerID,
		LabelColumn:     body.LabelColumn,
	})
	if err != nil {
		if errors.Is(err, services.ErrValidation) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, CreateDatasetResponse{
		DatasetID: result.DatasetID,
		Status:    result.Status,
		Message:   result.Message,
	})
}

func (h *DatasetHandler) listDatasets(w http.ResponseWriter, req *http.Request) {
	datasets, err := h.datasetSvc.ListReadyDatasets(req.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, datasets)
}

func (h *DatasetHandler) getDataset(w http.ResponseWriter, req *http.Request, id int) {
	result, err := h.datasetSvc.GetDataset(req.Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			http.Error(w, "dataset not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	// Backfill: datasets that became ready before the ML-package feature
	// existed get their package built on first view.
	if result.Status == "ready" && result.ExportStatus == "none" {
		if h.exportSvc.StartIfNeeded(id) {
			result.ExportStatus = "building"
			result.ExportProgress = 0
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// downloadExport streams the prebuilt ML package zip. While the package is
// still building (or failed), it answers with JSON state instead so the
// frontend can render the disabled button correctly even on a direct hit.
func (h *DatasetHandler) downloadExport(w http.ResponseWriter, req *http.Request, id int) {
	info, err := h.exportSvc.Info(req.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "dataset not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	switch info.Status {
	case "ready":
	case "building":
		writeJSON(w, http.StatusConflict, map[string]any{
			"status":   info.Status,
			"progress": info.Progress,
			"error":    "ML package is still being prepared",
		})
		return
	default: // none | error
		writeJSON(w, http.StatusConflict, map[string]any{
			"status": info.Status,
			"error":  "ML package is not available for this dataset",
		})
		return
	}

	file, err := os.Open(info.Path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "package file not found on disk"})
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "package file not readable"})
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.ZipName))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	if _, err := io.Copy(w, file); err != nil {
		fmt.Printf("[MLExport] streaming package for dataset %d failed: %v\n", id, err)
	}
}

func (h *DatasetHandler) updateDataset(w http.ResponseWriter, req *http.Request, id int) {
	claims, ok := auth.ClaimsFromContext(req.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var body UpdateDatasetRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	result, err := h.datasetSvc.UpdateDataset(req.Context(), services.UpdateDatasetInput{
		ID:              id,
		Name:            body.Name,
		ResearcherName:  body.ResearcherName,
		UserDescription: body.UserDescription,
		Tags:            body.Tags,
		LabelColumn:     body.LabelColumn,
	}, claims)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DatasetHandler) deleteDataset(w http.ResponseWriter, req *http.Request, id int) {
	claims, ok := auth.ClaimsFromContext(req.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	cleanup, err := h.datasetSvc.DeleteDataset(req.Context(), id, claims)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	fileIDs := make([]int, 0, len(cleanup))
	storagePaths := make([]string, 0, len(cleanup))
	for _, item := range cleanup {
		fileIDs = append(fileIDs, item.FileID)
		storagePaths = append(storagePaths, item.StoragePath)
	}
	h.fileHandler.CleanupDatasetFiles(fileIDs, storagePaths)
	h.exportSvc.Cleanup(id)
	w.WriteHeader(http.StatusNoContent)
}
