package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"dataset-platform/backend/internal/search"
)

type SearchHandler struct {
	searchSvc *search.Service
}

func NewSearchHandler(searchSvc *search.Service) *SearchHandler {
	return &SearchHandler{searchSvc: searchSvc}
}

func (h *SearchHandler) HandleSearch(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := req.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	// Note: query can be empty if user is only filtering by metadata (modality, type, etc)

	minSize, _ := strconv.ParseInt(firstQueryValue(q.Get("min_size"), q.Get("min_size_bytes")), 10, 64)
	maxSize, _ := strconv.ParseInt(firstQueryValue(q.Get("max_size"), q.Get("max_size_bytes")), 10, 64)
	minLabel, _ := strconv.ParseFloat(q.Get("min_label_completeness"), 64)
	maxLabel, _ := strconv.ParseFloat(q.Get("max_label_completeness"), 64)

	filters := search.SearchFilters{
		Modality:         q.Get("modality"),
		DatasetType:      q.Get("dataset_type"),
		AnnotationFormat: q.Get("annotation_format"),
		MinSizeBytes:     minSize,
		MaxSizeBytes:     maxSize,
		MinLabelComplete: minLabel,
		MaxLabelComplete: maxLabel,
		UploadedAfter:    parseDate(q.Get("uploaded_after")),
		UploadedBefore:   parseDate(q.Get("uploaded_before")),
	}

	results, err := h.searchSvc.Search(req.Context(), query, filters)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query":   query,
		"count":   len(results),
		"results": results,
	})
}

func firstQueryValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// parseDate parses a YYYY-MM-DD string into a UTC time.Time.
// Returns zero time if the string is empty or invalid.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
