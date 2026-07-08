package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

/*
This file is the single point of contact between our backend and the AI API.
*/

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	// MaxTokens limits the response length to prevent truncation.
	// We set this high enough to hold the full JSON analysis output.
	MaxTokens *int `json:"max_tokens,omitempty"`
}

type chatResponseChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatResponseChoice `json:"choices"`
}

// Client wraps all communication with the AI API.
type Client struct {
	baseURL             string // Chat/analysis API base URL
	chatAPIKey          string // Bearer token for chat API (empty = no auth, e.g. internal james)
	embeddingURL        string // OpenAI-compatible embedding API base (e.g. https://host/v1)
	embeddingModel      string // Embedding model name
	embeddingAPIKey     string // Bearer token for embedding API (empty = no auth)
	embeddingDimensions int    // Expected vector length; 0 = no validation
	model               string
	httpClient          *http.Client
}

func NewClient(baseURL, chatAPIKey, embeddingURL, embeddingModel, embeddingAPIKey string, embeddingDimensions int, model string) *Client {
	return &Client{
		baseURL:             strings.TrimRight(baseURL, "/"),
		chatAPIKey:          chatAPIKey,
		embeddingURL:        strings.TrimRight(embeddingURL, "/"),
		embeddingModel:      embeddingModel,
		embeddingAPIKey:     embeddingAPIKey,
		embeddingDimensions: embeddingDimensions,
		model:               model,
		httpClient: &http.Client{
			Timeout: 3 * time.Minute,
		},
	}
}

// complete sends a single-turn chat completion to Flash and returns a response text.
func (c *Client) complete(ctx context.Context, userPrompt string) (string, error) {
	maxTokens := 4096
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.7,
		MaxTokens:   &maxTokens,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.chatAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.chatAPIKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("AI API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI API returned status %d: %s", resp.StatusCode, string(b))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("Failed to decode AI API response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("AI API returned no choices")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("AI API returned empty content")
	}

	return content, nil
}
