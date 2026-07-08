package search

import (
	"context"
	"fmt"
)

// filterOnlySearch applies only metadata filters without any text search.
// Used when the user provides only filters and no search query.
func (s *Service) filterOnlySearch(ctx context.Context, filters SearchFilters) ([]SearchResult, error) {
	sql := `
		SELECT
			d.id, d.name, d.researcher_name, coalesce(d.ai_summary,''),
			coalesce(d.modality,''), coalesce(d.dataset_type,''),
			coalesce(d.annotation_format,''), d.label_completeness,
			d.total_size_bytes, coalesce(d.tags, '{}'), d.uploaded_at,
			0.0 AS score
		FROM datasets d
		WHERE d.status = 'ready'`

	args := []any{}
	argIdx := 1

	sql, args, argIdx = applyFilters(sql, args, argIdx, filters)
	sql += " ORDER BY d.uploaded_at DESC LIMIT 100"

	return s.queryResults(ctx, sql, args)
}

// applyFilters appends WHERE clauses for active filters.
func applyFilters(sql string, args []any, argIdx int, f SearchFilters) (string, []any, int) {
	if f.Modality != "" {
		sql += fmt.Sprintf(" AND lower(d.modality) = lower($%d)", argIdx)
		args = append(args, f.Modality)
		argIdx++
	}
	if f.DatasetType != "" {
		sql += fmt.Sprintf(" AND lower(d.dataset_type) = lower($%d)", argIdx)
		args = append(args, f.DatasetType)
		argIdx++
	}
	if f.AnnotationFormat != "" {
		sql += fmt.Sprintf(" AND lower(d.annotation_format) LIKE '%%' || lower($%d) || '%%'", argIdx)
		args = append(args, f.AnnotationFormat)
		argIdx++
	}
	if f.MinSizeBytes > 0 {
		sql += fmt.Sprintf(" AND d.total_size_bytes >= $%d", argIdx)
		args = append(args, f.MinSizeBytes)
		argIdx++
	}
	if f.MaxSizeBytes > 0 {
		sql += fmt.Sprintf(" AND d.total_size_bytes <= $%d", argIdx)
		args = append(args, f.MaxSizeBytes)
		argIdx++
	}
	if f.MinLabelComplete > 0 {
		sql += fmt.Sprintf(" AND d.label_completeness >= $%d", argIdx)
		args = append(args, f.MinLabelComplete)
		argIdx++
	}
	if f.MaxLabelComplete > 0 && f.MaxLabelComplete < 1.0 {
		sql += fmt.Sprintf(" AND d.label_completeness <= $%d", argIdx)
		args = append(args, f.MaxLabelComplete)
		argIdx++
	}
	if !f.UploadedAfter.IsZero() {
		sql += fmt.Sprintf(" AND d.uploaded_at >= $%d", argIdx)
		args = append(args, f.UploadedAfter)
		argIdx++
	}
	if !f.UploadedBefore.IsZero() {
		sql += fmt.Sprintf(" AND d.uploaded_at <= $%d", argIdx)
		args = append(args, f.UploadedBefore)
		argIdx++
	}
	return sql, args, argIdx
}
