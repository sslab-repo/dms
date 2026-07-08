package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAI-compatible embeddings API wire types.
// Spec: POST /v1/embeddings  {"model": "...", "input": "..."}
// Response: {"data": [{"embedding": [...], "index": 0}]}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingResponse struct {
	Data []embeddingData `json:"data"`
}

func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	reqBody := embeddingRequest{
		Model: c.embeddingModel,
		Input: text,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.embeddingURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.embeddingAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.embeddingAPIKey)
	}

	embClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := embClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(b))
	}

	var embResp embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, err
	}
	if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding API returned empty embedding")
	}

	vec := embResp.Data[0].Embedding
	if c.embeddingDimensions > 0 && len(vec) != c.embeddingDimensions {
		return nil, fmt.Errorf("embedding dimension mismatch: expected %d, got %d (wrong model?)", c.embeddingDimensions, len(vec))
	}

	return vec, nil
}
