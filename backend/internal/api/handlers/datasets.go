package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"dataset-platform/backend/internal/auth"
	"dataset-platform/backend/internal/filehandler"
	"dataset-platform/backend/internal/services"
)

type DatasetHandler struct {
	datasetSvc  *services.DatasetService
	fileHandler *filehandler.Handler
}

func NewDatasetHandler(datasetSvc *services.DatasetService, fileHandler *filehandler.Handler) *DatasetHandler {
	return &DatasetHandler{datasetSvc: datasetSvc, fileHandler: fileHandler}
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

	writeJSON(w, http.StatusOK, result)
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
	w.WriteHeader(http.StatusNoContent)
}
