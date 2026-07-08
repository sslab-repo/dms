package handlers

import (
	"errors"
	"net/http"

	"dataset-platform/backend/internal/services"
)

func writeMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrValidation):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, services.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, services.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you do not have permission to modify this dataset"})
	case errors.Is(err, services.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
