package search

import (
	"context"
	"database/sql"
	"strings"

	"dataset-platform/backend/internal/ai"
)

// Service encapsulates the search functionality.
type Service struct {
	db                          *sql.DB
	aiClient                    *ai.Client
	semanticSimilarityThreshold float64
}

// NewService creates a new search service.
func NewService(db *sql.DB, aiClient *ai.Client, semanticSimilarityThreshold float64) *Service {
	return &Service{
		db:                          db,
		aiClient:                    aiClient,
		semanticSimilarityThreshold: semanticSimilarityThreshold,
	}
}

// Search executes a hybrid search with the given query and filters.
func (s *Service) Search(ctx context.Context, query string, filters SearchFilters) ([]SearchResult, error) {
	return s.runHybrid(ctx, strings.TrimSpace(query), filters)
}
