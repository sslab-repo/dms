package search

import (
	"context"
	"fmt"
)

// keywordSearch uses PostgreSQL full-text search over precise structured
// fields plus human and AI-written descriptive text.
func (s *Service) keywordSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	baseSQL := `
		WITH query AS (
			SELECT websearch_to_tsquery('english', $1) AS q
		)
		SELECT
			d.id, d.name, d.researcher_name, coalesce(d.ai_summary,'') AS ai_summary,
			coalesce(d.modality,''), coalesce(d.dataset_type,''), coalesce(d.annotation_format,''),
			d.label_completeness, d.total_size_bytes, coalesce(d.tags, '{}'), d.uploaded_at,
			ts_rank_cd(d.search_vector, query.q) AS score
		FROM datasets d
		CROSS JOIN query
		WHERE d.status = 'ready'
		  AND d.search_vector @@ query.q`

	args := []any{query}
	argIdx := 2

	baseSQL, args, argIdx = applyFilters(baseSQL, args, argIdx, filters)
	baseSQL += fmt.Sprintf(" ORDER BY score DESC LIMIT %d", limit)

	results, err := s.queryResults(ctx, baseSQL, args)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].KeywordScore = results[i].FusionScore
	}
	return results, nil
}
