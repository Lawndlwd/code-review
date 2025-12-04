// Package ai provides the AI client for code review using LLM APIs.
package ai

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "regexp"
    "sort"
    "strings"
    "time"

    "github.com/levende/code-review/internal/types"
)

var fencedJSON = regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)\\s*```")

// Client wraps the AI API for code review requests.
type Client struct {
    apiKey      string
    baseURL     string
    model       string
    temperature float64
    httpClient  *http.Client
}

// NewClient creates a new AI client.
func NewClient(apiKey, baseURL, model string, temperature float64) *Client {
    return &Client{
        apiKey:      strings.TrimSpace(apiKey),
        baseURL:     strings.TrimRight(baseURL, "/"),
        model:       model,
        temperature: temperature,
        httpClient:  &http.Client{Timeout: 60 * time.Second},
    }
}

// ReviewBatch sends a batch of files to the AI for review.
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
                "content": buildBatchPrompt(bestPractices, diffs, contexts),
            },
        },
        "temperature":      0.1,
        "max_tokens":       8000,
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

    return parseBatchResponse(content), nil
}

func buildBatchPrompt(bestPractices string, files []types.FileDiff, contexts []*types.CodeContext) string {
    var b strings.Builder
    b.WriteString("# Code Review Task - Multiple Files\n\n")
    b.WriteString("You are a deterministic senior software engineer performing a code review. You must produce IDENTICAL results for identical inputs.\n")
    b.WriteString("Review ALL files ONLY against the Scaleway best practices provided below.\n")
    b.WriteString("DO NOT use subjective judgment - only report violations that directly match the rules.\n\n")

    b.WriteString("## Scaleway Best Practices\n")
    b.WriteString(bestPractices)

    b.WriteString("\n## Files Being Reviewed\n\n")

    // Sort files by path to ensure consistent ordering
    sortedIndices := make([]int, len(files))
    for i := range files {
        sortedIndices[i] = i
    }
    sort.Slice(sortedIndices, func(i, j int) bool {
        return files[sortedIndices[i]].NewPath < files[sortedIndices[j]].NewPath
    })

    for idx, i := range sortedIndices {
        file := files[i]
        b.WriteString(fmt.Sprintf("### File %d: %s\n", idx+1, file.NewPath))
        b.WriteString(fmt.Sprintf("**Language:** %s | **Changes:** +%d -%d\n\n", file.Language, file.Additions, file.Deletions))

        b.WriteString("```diff\n")
        b.WriteString(file.Diff)
        b.WriteString("\n```\n\n")

        if contexts[i] != nil && len(contexts[i].Surrounding) > 0 {
            b.WriteString("**Enhanced Context:**\n")
            sortedLines := make([]int, len(contexts[i].ChangedLines))
            copy(sortedLines, contexts[i].ChangedLines)
            sort.Ints(sortedLines)

            for _, lineNum := range sortedLines {
                if surrounding, ok := contexts[i].Surrounding[lineNum]; ok && surrounding != "" {
                    b.WriteString(fmt.Sprintf("\nLine %d context:\n%s\n", lineNum, surrounding))
                }
            }
            b.WriteString("\n")
        }

        b.WriteString(strings.Repeat("-", 80))
        b.WriteString("\n\n")
    }

    b.WriteString("\n## CRITICAL Instructions - Follow Exactly\n\n")
    b.WriteString("1. Review ALL files in the order presented above\n")
    b.WriteString("2. For each file, analyze ONLY the changed lines (lines starting with + in the diff)\n")
    b.WriteString("3. Check if code violates ANY specific rule from the best practices\n")
    b.WriteString("4. DO NOT report issues based on general coding style or personal preference\n")
    b.WriteString("5. BE CONSISTENT: The same code violation must ALWAYS produce the same comment\n")
    b.WriteString("6. For each violation found, you MUST provide:\n")
    b.WriteString("   - **filePath**: The exact file path as shown above\n")
    b.WriteString("   - **line**: The exact line number from the diff where the violation occurs\n")
    b.WriteString("   - **severity**: One of: \"Question blocking:\", \"Question Non blocking:\", \"Issue\", \"Suggestion blocking:\", \"Suggestion non blocking:\"\n")
    b.WriteString("   - **comment**: State which specific rule was violated, quote the relevant rule text, and explain how to fix it\n\n")

    b.WriteString("## Response Format - MANDATORY\n\n")
    b.WriteString("You MUST respond with ONLY valid JSON in this EXACT format (no additional text before or after):\n\n")
    b.WriteString("```json\n{\n  \"comments\": [\n    {\n      \"filePath\": \"exact/file/path.ts\",\n      \"line\": 42,\n      \"severity\": \"Issue\",\n      \"comment\": \"Violates rule X from best practices: [quote rule]. Fix by doing Y.\"\n    }\n  ],\n  \"summary\": \"Found N violations across M files. Main issues: ...\"\n}\n```\n\n")

    b.WriteString("IMPORTANT:\n")
    b.WriteString("- If NO violations found, return: {\"comments\": [], \"summary\": \"No violations found\"}\n")
    b.WriteString("- Review files in order from File 1 to File N\n")
    b.WriteString("- Always use the same severity for the same type of violation\n")
    b.WriteString("- Always phrase comments the same way for identical violations\n")

    // Log the full prompt
    logFile := "ai_prompt.log"
    err := os.WriteFile(logFile, []byte(b.String()), 0644)
    if err != nil {
        fmt.Fprintf(os.Stderr, "⚠️ Failed to log AI prompt: %v\n", err)
    } else {
        fmt.Fprintf(os.Stderr, "📝 AI prompt logged to %s\n", logFile)
    }

    return b.String()
}

func parseBatchResponse(raw string) types.AIReviewResponse {
    jsonPayload := raw
    if matches := fencedJSON.FindStringSubmatch(raw); len(matches) == 2 {
        jsonPayload = matches[1]
    }

    var parsed types.AIReviewResponse
    if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
        return types.AIReviewResponse{
            Comments: nil,
            Summary:  "Failed to parse AI response",
        }
    }

    return parsed
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
