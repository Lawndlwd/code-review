package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lawndlwd/golum/internal/types"
)

type Client struct {
	apiKey      string
	baseURL     string
	model       string
	temperature float64
	httpClient  *http.Client
}

func NewClient(apiKey, baseURL, model string, temperature float64) *Client {
	return &Client{
		apiKey:      strings.TrimSpace(apiKey),
		baseURL:     strings.TrimRight(baseURL, "/"),
		model:       model,
		temperature: temperature,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) ReviewBatch(ctx context.Context, bestPractices string, diffs []types.FileDiff, contexts []*types.CodeContext) (types.AIReviewResponse, error) {
	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a deterministic senior software engineer performing a code review. You must produce IDENTICAL results for identical inputs.",
			},
			{
				"role":    "user",
				"content": BuildBatchPrompt(bestPractices, diffs, contexts),
			},
		},
		"temperature":      0.1,  // Force deterministic output
		"max_tokens":       8000, // Increased for batch processing
		"presence_penalty": 0.0,
		"top_p":            0.5,
		"seed":             1234,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.AIReviewResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return types.AIReviewResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return types.AIReviewResponse{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		return types.AIReviewResponse{}, fmt.Errorf("ai request failed: %s - %s", resp.Status, buf.String())
	}

	var parsed completionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return types.AIReviewResponse{}, fmt.Errorf("decode response: %w", err)
	}

	content := parsed.FirstContent()
	if content == "" {
		return types.AIReviewResponse{}, fmt.Errorf("empty AI response")
	}

	return ParseBatchResponse(content), nil
}

type completionsResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c completionsResponse) FirstContent() string {
	if len(c.Choices) == 0 {
		return ""
	}
	return c.Choices[0].Message.Content
}
