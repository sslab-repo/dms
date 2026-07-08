package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
RewriteQuery is called at search time when initial results are weak.

It takes the original query and asks Flash to rephrase it using
more precise terminology that is more likely to match dataset
descriptions and labels.
*/
func (c *Client) RewriteQuery(ctx context.Context, originalQuery string) (string, error) {
	prompt := fmt.Sprintf(`You are a search query optimizer for a scientific dataset repository.

		A researcher searched for: "%s"

		The initial results were weak. Rewrite this query to be more precise and likely to match
		dataset titles, descriptions, and labels in a research data repository.
		Return ONLY the rewritten query text. No explanation.
	`, originalQuery)

	rewritten, err := c.complete(ctx, prompt)
	if err != nil {
		return originalQuery, err // fall back to original on error
	}

	rewritten = strings.TrimSpace(rewritten)
	if rewritten == "" {
		return originalQuery, nil
	}
	return rewritten, nil
}

/*
RerankResults is called after fusion when we want Flash to do a
final quality pass over the top-N candidates.

candidates is a JSON-encoded list of dataset summaries with their IDs.
Flash returns a reordered list of IDs.
*/
type RerankCandidate struct {
	ID      int    `json:"id"`
	Summary string `json:"summary"`
}

func (c *Client) RerankResults(ctx context.Context, query string, candidates []RerankCandidate) ([]int, error) {
	candidateJSON, _ := json.Marshal(candidates)

	prompt := fmt.Sprintf(`You are a search result ranker for a scientific dataset repository.

		Query: "%s"

		Candidates (JSON array with id and summary):
		%s

		Rerank these candidates so the most relevant to the query comes first.
		Return ONLY a JSON array of IDs in your preferred order. Example: [3, 1, 4, 2]
		No explanation.
	`, query, string(candidateJSON))

	raw, err := c.complete(ctx, prompt)
	if err != nil {
		// On error return original order
		ids := make([]int, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		return ids, nil
	}

	raw = strings.TrimSpace(stripMarkdownFences(raw))
	var ids []int
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		// Fall back to original order
		ids = make([]int, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
	}
	return ids, nil
}
