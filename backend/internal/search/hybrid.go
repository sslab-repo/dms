package search

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// runHybrid executes keyword and semantic search in parallel, fuses results.
// If query is empty, only applies filters without text search.
func (s *Service) runHybrid(ctx context.Context, query string, filters SearchFilters) ([]SearchResult, error) {
	type rankMap map[int]int // datasetID -> rank (1-based)

	// If query is empty, just return filtered results without keyword/semantic search
	if query == "" {
		return s.filterOnlySearch(ctx, filters)
	}

	type searchResponse struct {
		results []SearchResult
		err     error
	}

	kwCh := make(chan searchResponse, 1)
	semCh := make(chan searchResponse, 1)

	go func() {
		results, err := s.keywordSearch(ctx, query, filters, 50)
		kwCh <- searchResponse{results: results, err: err}
	}()
	go func() {
		results, err := s.semanticSearch(ctx, query, filters, 50)
		semCh <- searchResponse{results: results, err: err}
	}()

	kwResp := <-kwCh
	semResp := <-semCh

	kwResults, kwErr := kwResp.results, kwResp.err
	if kwErr != nil {
		fmt.Printf("keyword search error: %v\n", kwErr)
		kwResults = nil
	}

	kwRanks := make(rankMap)
	for i, r := range kwResults {
		kwRanks[r.DatasetID] = i + 1
	}

	semResults, semErr := semResp.results, semResp.err
	if semErr != nil {
		fmt.Printf("semantic search error (non-fatal): %v\n", semErr)
		semResults = nil
	}

	semRanks := make(rankMap)
	for i, r := range semResults {
		semRanks[r.DatasetID] = i + 1
	}

	seen := make(map[int]bool)
	var allIDs []int
	for _, r := range kwResults {
		if !seen[r.DatasetID] {
			seen[r.DatasetID] = true
			allIDs = append(allIDs, r.DatasetID)
		}
	}
	for _, r := range semResults {
		if !seen[r.DatasetID] {
			seen[r.DatasetID] = true
			allIDs = append(allIDs, r.DatasetID)
		}
	}

	if len(allIDs) == 0 {
		return []SearchResult{}, nil
	}

	const k = 60.0

	type fusedEntry struct {
		datasetID int
		score     float64
		inKW      bool
		inSem     bool
	}
	fused := make(map[int]*fusedEntry)

	for id := range seen {
		fused[id] = &fusedEntry{datasetID: id}
	}

	for id, rank := range kwRanks {
		fused[id].score += 1.0 / (k + float64(rank))
		fused[id].inKW = true
	}
	for id, rank := range semRanks {
		fused[id].score += 1.0 / (k + float64(rank))
		fused[id].inSem = true
	}

	entries := make([]*fusedEntry, 0, len(fused))
	for _, e := range fused {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	topN := 20
	if len(entries) < topN {
		topN = len(entries)
	}
	entries = entries[:topN]

	resultsByID := make(map[int]SearchResult)
	for _, r := range kwResults {
		resultsByID[r.DatasetID] = r
	}
	for _, r := range semResults {
		if existing, exists := resultsByID[r.DatasetID]; exists {
			existing.SemanticScore = r.SemanticScore
			resultsByID[r.DatasetID] = existing
		} else {
			resultsByID[r.DatasetID] = r
		}
	}

	var finalResults []SearchResult
	for _, e := range entries {
		base, ok := resultsByID[e.datasetID]
		if !ok {
			continue
		}

		switch {
		case e.inKW && e.inSem:
			base.Citation = CitationHybrid
		case e.inKW:
			base.Citation = CitationKeyword
		default:
			base.Citation = CitationSemantic
		}
		base.FusionScore = math.Round(e.score*10000) / 10000

		finalResults = append(finalResults, base)
	}

	return finalResults, nil
}
