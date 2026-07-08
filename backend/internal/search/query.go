package search

import (
	"context"

	"github.com/lib/pq"
)

// queryResults is the shared row scanner used by both search paths.
func (s *Service) queryResults(ctx context.Context, sqlStr string, args []any) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var tags []string
		err := rows.Scan(
			&r.DatasetID, &r.Name, &r.ResearcherName, &r.AISummary,
			&r.Modality, &r.DatasetType, &r.AnnotationFormat,
			&r.LabelCompleteness, &r.TotalSizeBytes, pq.Array(&tags),
			&r.UploadedAt, &r.FusionScore,
		)
		if err != nil {
			continue
		}
		r.Tags = tags
		results = append(results, r)
	}
	return results, nil
}
