package search

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// semanticSearch encodes the query via the embedding API,
// fetches stored embeddings from PostgreSQL, and ranks by cosine similarity.
func (s *Service) semanticSearch(ctx context.Context, query string, filters SearchFilters, limit int) ([]SearchResult, error) {
	queryVec, err := s.aiClient.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	candidateSQL := `SELECT d.id, d.embedding_json FROM datasets d WHERE d.status='ready' AND d.embedding_json IS NOT NULL`
	args := []any{}
	argIdx := 1
	candidateSQL, args, _ = applyFilters(candidateSQL, args, argIdx, filters)

	rows, err := s.db.QueryContext(ctx, candidateSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct {
		id         int
		similarity float64
	}
	var candidates []candidate

	for rows.Next() {
		var id int
		var embJSON string
		if err := rows.Scan(&id, &embJSON); err != nil {
			fmt.Printf("[SemanticSearch] scan error: %v\n", err)
			continue
		}

		var vec []float64
		if err := json.Unmarshal([]byte(embJSON), &vec); err != nil {
			continue
		}
		sim := cosineSimilarity(queryVec, vec)
		if !passesSemanticThreshold(sim, s.semanticSimilarityThreshold) {
			continue
		}
		candidates = append(candidates, candidate{id, sim})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("semantic search db iteration error: %w", err)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	if len(candidates) == 0 {
		return []SearchResult{}, nil
	}

	ids := make([]int, len(candidates))
	for i, c := range candidates {
		ids[i] = c.id
	}

	placeholders := ""
	args = []any{}
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}

	querySQL := fmt.Sprintf(`
		SELECT d.id, d.name, d.researcher_name, coalesce(d.ai_summary,''),
			coalesce(d.modality,''), coalesce(d.dataset_type,''),
			coalesce(d.annotation_format,''), d.label_completeness,
			d.total_size_bytes, coalesce(d.tags, '{}'), d.uploaded_at,
			0.0 AS score
		FROM datasets d
		WHERE d.id IN (%s)`, placeholders)

	results, err := s.queryResults(ctx, querySQL, args)
	if err != nil {
		return nil, err
	}

	byID := make(map[int]SearchResult, len(results))
	for _, r := range results {
		byID[r.DatasetID] = r
	}
	ordered := make([]SearchResult, 0, len(candidates))
	for _, c := range candidates {
		if r, ok := byID[c.id]; ok {
			r.SemanticScore = math.Round(c.similarity*10000) / 10000
			r.FusionScore = r.SemanticScore
			ordered = append(ordered, r)
		}
	}

	return ordered, nil
}

func passesSemanticThreshold(similarity, threshold float64) bool {
	return similarity >= threshold
}
