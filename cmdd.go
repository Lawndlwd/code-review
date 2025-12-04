// Single-file version of the code-review CLI with Tree-sitter integration.
// This file is self-contained and can be built directly.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	"github.com/spf13/pflag"
)

// ===== Types (was internal/types) =====

type FileDiff struct {
	OldPath   string
	NewPath   string
	Diff      string
	Additions int
	Deletions int
	Language  string
}

type ReviewComment struct {
	FilePath string `json:"filePath"`
	Line     int    `json:"line"`
	Comment  string `json:"comment"`
	Severity string `json:"severity"`
}

type AIReviewResponse struct {
	Comments []ReviewComment `json:"comments"`
	Summary  string          `json:"summary"`
}

// ===== Tree-sitter Parser =====

type CodeContext struct {
	ChangedLines []int            // Line numbers that were changed
	Surrounding  map[int]string   // Line number -> surrounding context (5 lines before/after)
}

type Parser struct {
	jsParser  *tree_sitter.Parser
	tsParser  *tree_sitter.Parser
	tsxParser *tree_sitter.Parser
}

func NewParser() *Parser {
	return &Parser{
		jsParser:  tree_sitter.NewParser(),
		tsParser:  tree_sitter.NewParser(),
		tsxParser: tree_sitter.NewParser(),
	}
}

func (p *Parser) Init() error {
	lang := tree_sitter.NewLanguage(tree_sitter_javascript.Language())

	p.jsParser.SetLanguage(lang)
	// For now use the JavaScript grammar for TS/TSX as well
	p.tsParser.SetLanguage(lang)
	p.tsxParser.SetLanguage(lang)

	return nil
}

func (p *Parser) Close() {
	if p.jsParser != nil {
		p.jsParser.Close()
	}
	if p.tsParser != nil {
		p.tsParser.Close()
	}
	if p.tsxParser != nil {
		p.tsxParser.Close()
	}
}

func (p *Parser) getParserForFile(filename string) *tree_sitter.Parser {
	switch {
	case strings.HasSuffix(filename, ".tsx"):
		return p.tsxParser
	case strings.HasSuffix(filename, ".ts"):
		return p.tsParser
	case strings.HasSuffix(filename, ".jsx"):
		return p.jsParser
	case strings.HasSuffix(filename, ".js"):
		return p.jsParser
	default:
		return nil
	}
}

// ===== Enhanced Diff Processing =====

func parseChangedLines(diff string) []int {
	var changedLines []int
	lines := strings.Split(diff, "\n")
	currentLine := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			// Parse hunk header: @@ -start,count +start,count @@
			re := regexp.MustCompile(`@@ -\d+,\d+ \+(\d+),\d+ @@`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				if start, err := strconv.Atoi(matches[1]); err == nil {
					currentLine = start
				}
			}
			continue
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			changedLines = append(changedLines, currentLine)
			currentLine++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			// Skip decrement for deletions
		} else if strings.HasPrefix(line, " ") {
			currentLine++
		}
	}

	return changedLines
}

func (p *Parser) AnalyzeCodeContext(fileContent string, changedLines []int, filename string) *CodeContext {
	context := &CodeContext{
		ChangedLines: changedLines,
		Surrounding:  make(map[int]string),
	}

	parser := p.getParserForFile(filename)
	if parser == nil {
		for _, lineNum := range changedLines {
			context.Surrounding[lineNum] = getSurroundingLines(fileContent, lineNum, 5)
		}
		return context
	}

	// Always provide simple surrounding context per changed line
	for _, lineNum := range changedLines {
		context.Surrounding[lineNum] = getSurroundingLines(fileContent, lineNum, 5)
	}

	return context
}

func getSurroundingLines(content string, lineNum, contextLines int) string {
	lines := strings.Split(content, "\n")
	start := max(0, lineNum-contextLines-1)
	end := min(len(lines), lineNum+contextLines)

	if start >= len(lines) || end <= 0 {
		return ""
	}

	var result []string
	for i := start; i < end; i++ {
		prefix := "    "
		if i == lineNum-1 {
			prefix = ">>> "
		}
		result = append(result, fmt.Sprintf("%s%4d: %s", prefix, i+1, lines[i]))
	}

	return strings.Join(result, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ===== Enhanced FileDiff with Context =====

func getFileContent(repoPath, filePath, ref string) (string, error) {
	// Try to get content from git
	cmd := exec.Command("git", "-C", repoPath, "show", fmt.Sprintf("%s:%s", ref, filePath))
	output, err := cmd.Output()
	if err != nil {
		// Fallback to reading from file system
		fullPath := filepath.Join(repoPath, filePath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}
	return string(output), nil
}

func enrichDiffWithContext(repoPath string, diff FileDiff, targetBranch string) (FileDiff, *CodeContext, error) {
	// Parse changed lines from diff
	changedLines := parseChangedLines(diff.Diff)

	// Get current file content
	currentContent, err := getFileContent(repoPath, diff.NewPath, "HEAD")
	if err != nil {
		return diff, nil, err
	}

	// Determine language (for prompt decoration only)
	switch {
	case strings.HasSuffix(diff.NewPath, ".tsx"):
		diff.Language = "tsx"
	case strings.HasSuffix(diff.NewPath, ".ts"):
		diff.Language = "typescript"
	case strings.HasSuffix(diff.NewPath, ".jsx"):
		diff.Language = "jsx"
	case strings.HasSuffix(diff.NewPath, ".js"):
		diff.Language = "javascript"
	}

	parser := NewParser()
	if err := parser.Init(); err != nil {
		return diff, nil, err
	}
	defer parser.Close()

	ctx := parser.AnalyzeCodeContext(currentContent, changedLines, diff.NewPath)
	return diff, ctx, nil
}

// ===== Enhanced AI client =====

type AIClient struct {
	apiKey      string
	baseURL     string
	model       string
	temperature float64
	httpClient  *http.Client
}

func NewAIClient(apiKey, baseURL, model string, temperature float64) *AIClient {
	return &AIClient{
		apiKey:      strings.TrimSpace(apiKey),
		baseURL:     strings.TrimRight(baseURL, "/"),
		model:       model,
		temperature: temperature,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *AIClient) ReviewBatch(ctx context.Context, bestPractices string, diffs []FileDiff, contexts []*CodeContext) (AIReviewResponse, error) {
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
		"temperature":      0.1,  // Force deterministic output
		"max_tokens":       8000, // Increased for batch processing
		"presence_penalty": 0.0,
		"top_p":            0.5,
		"seed":             1234,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return AIReviewResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return AIReviewResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AIReviewResponse{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		return AIReviewResponse{}, fmt.Errorf("ai request failed: %s - %s", resp.Status, buf.String())
	}

	var parsed completionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return AIReviewResponse{}, fmt.Errorf("decode response: %w", err)
	}

	content := parsed.FirstContent()
	if content == "" {
		return AIReviewResponse{}, fmt.Errorf("empty AI response")
	}

	return parseBatchResponse(content), nil
}

func buildBatchPrompt(bestPractices string, files []FileDiff, contexts []*CodeContext) string {
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
			// Sort line numbers for consistent ordering
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
	b.WriteString("   - **filePath**: The exact file path as shown above (e.g., \"src/components/Button.tsx\")\n")
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

	// ✅ Log the full prompt to a file
	logFile := "ai_prompt.log"
	err := os.WriteFile(logFile, []byte(b.String()), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ Failed to log AI prompt: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "📝 AI prompt logged to %s\n", logFile)
	}

	return b.String()
}

func parseBatchResponse(raw string) AIReviewResponse {
	jsonPayload := raw
	if matches := fencedJSON.FindStringSubmatch(raw); len(matches) == 2 {
		jsonPayload = matches[1]
	}

	var parsed AIReviewResponse
	if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
		return AIReviewResponse{
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


var fencedJSON = regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)\\s*```")

// ===== Best practices loader (was internal/bestpractices) =====

func LoadBestPractices(directory string) (string, error) {
	pattern := filepath.Join(directory, "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob markdown: %w", err)
	}

	sort.Strings(matches)

	var builder strings.Builder

	for _, file := range matches {
		content, readErr := os.ReadFile(file)
		if readErr != nil {
			return "", fmt.Errorf("read %s: %w", file, readErr)
		}

		base := filepath.Base(file)
		title := strings.TrimSuffix(base, filepath.Ext(base))
		title = strings.TrimSpace(splitCamelCase(title))

		builder.WriteString("\n# ")
		builder.WriteString(title)
		builder.WriteString("\n\n")
		builder.Write(content)
		builder.WriteString("\n\n")
	}

	if builder.Len() == 0 {
		return "", fmt.Errorf("no markdown guidelines found in %s", directory)
	}

	return builder.String(), nil
}

func splitCamelCase(input string) string {
	var result []rune
	for idx, r := range input {
		if idx > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, ' ')
		}
		result = append(result, r)
	}
	return string(result)
}

// ===== Filter (was internal/filter) =====

func FilterEligible(files []FileDiff, limit int) []FileDiff {
	var result []FileDiff

	for _, diff := range files {
		path := diff.NewPath
		if path == "" {
			path = diff.OldPath
		}
		if shouldSkip(path) {
			continue
		}
		result = append(result, diff)
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result
}

func shouldSkip(path string) bool {
	if path == "" {
		return true
	}
	if hasAnySuffix(path, ".md", ".json") {
		return true
	}
	if containsAny(path, "node_modules", "dist", ".gitlab") {
		return true
	}
	// Only process JS/TS files for Tree-sitter parsing
	return !(hasAnySuffix(path, ".ts", ".tsx", ".js", ".jsx"))
}

func hasAnySuffix(path string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}

func containsAny(path string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(path, needle) {
			return true
		}
	}
	return false
}

// ===== Git local diff (was internal/git) =====

type GitLocalOptions struct {
	RepoPath        string
	BaseRef         string
	TargetBranch    string // Add this
	IncludeUnstaged bool
	Local           bool // ADD THIS LINE
}

func GitLocalChanges(opts GitLocalOptions) ([]FileDiff, error) {
	repo := filepath.Clean(opts.RepoPath)
	// Use target branch if specified, otherwise compare to HEAD
	var compareRef string
	if opts.TargetBranch != "" && opts.TargetBranch != "HEAD" {
		compareRef = opts.TargetBranch
	} else {
		compareRef = "HEAD"
	}
	files, err := changedFiles(repo, compareRef, opts.IncludeUnstaged, opts.TargetBranch, opts.Local)
	if err != nil {
		return nil, err
	}
	var diffs []FileDiff
	for _, file := range files {
		diffText, err := diffFile(repo, compareRef, file, opts.IncludeUnstaged, opts.TargetBranch, opts.Local)
		if err != nil || strings.TrimSpace(diffText) == "" {
			continue
		}
		additions := countPrefix(diffText, '+')
		deletions := countPrefix(diffText, '-')
		diffs = append(diffs, FileDiff{
			OldPath:   file,
			NewPath:   file,
			Diff:      diffText,
			Additions: additions,
			Deletions: deletions,
		})
	}
	return diffs, nil
}

func changedFiles(repo, compareRef string, includeUnstaged bool, targetBranch string, local bool) ([]string, error) {
	var stdout []byte
	var err error

	if local {
		// Compare working directory + staged changes to origin/targetBranch
		originBranch :=  targetBranch
		args := []string{"-C", repo, "diff", "--name-only", originBranch}
		stdout, err = exec.Command("git", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("git diff %s: %w", originBranch, err)
		}
	} else if targetBranch != "" && targetBranch != "HEAD" {
		// Find the merge base (where your branch diverged from target)
		mergeBaseCmd := exec.Command("git", "-C", repo, "merge-base", targetBranch, "HEAD")
		mergeBase, err := mergeBaseCmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git merge-base: %w", err)
		}
		baseCommit := strings.TrimSpace(string(mergeBase))

		// Now diff from that merge base to HEAD (only YOUR changes)
		args := []string{"-C", repo, "diff", "--name-only", baseCommit, "HEAD"}
		stdout, err = exec.Command("git", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("git diff %s HEAD: %w", baseCommit, err)
		}
	} else {
		args := []string{"-C", repo, "diff", "--name-only", compareRef}
		stdout, err = exec.Command("git", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("git diff --name-only: %w", err)
		}
	}

	entries := parseLines(stdout)
	if includeUnstaged {
		cachedArgs := []string{"-C", repo, "diff", "--cached", "--name-only"}
		cached, err := exec.Command("git", cachedArgs...).Output()
		if err == nil {
			entries = append(entries, parseLines(cached)...)
		}
	}
	seen := make(map[string]struct{})
	var unique []string
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		unique = append(unique, entry)
	}
	return unique, nil
}

func diffFile(repo, compareRef, path string, includeUnstaged bool, targetBranch string, local bool) (string, error) {
	var buf bytes.Buffer
	if includeUnstaged {
		args := []string{"-C", repo, "diff", "--cached", compareRef, "--", path}
		if out, err := exec.Command("git", args...).Output(); err == nil {
			buf.Write(out)
		}
	}

	var args []string
	if local {
		// Compare working directory to origin/targetBranch
		originBranch := targetBranch
		args = []string{"-C", repo, "diff", originBranch, "--", path}
	} else if targetBranch != "" && targetBranch != "HEAD" {
		// Find merge base
		mergeBaseCmd := exec.Command("git", "-C", repo, "merge-base", targetBranch, "HEAD")
		mergeBase, err := mergeBaseCmd.Output()
		if err != nil {
			return "", fmt.Errorf("git merge-base: %w", err)
		}
		baseCommit := strings.TrimSpace(string(mergeBase))

		// Diff from merge base to HEAD
		args = []string{"-C", repo, "diff", baseCommit, "HEAD", "--", path}
	} else {
		args = []string{"-C", repo, "diff", compareRef, "--", path}
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s: %w", path, err)
	}
	buf.Write(out)
	return buf.String(), nil
}

func parseLines(input []byte) []string {
	raw := strings.Split(string(input), "\n")
	var result []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func countPrefix(diff string, prefix rune) int {
	count := 0
	for _, line := range strings.Split(diff, "\n") {
		if len(line) > 0 && rune(line[0]) == prefix {
			count++
		}
	}
	return count
}

// ===== Main CLI (was cmd/review) =====

type config struct {
	AIToken      string
	AIEndpoint   string
	AIModel      string
	Temperature  float64
	Guidelines   string
	RepoPath     string
	TargetBranch string
	UseTreeSitter bool
	Local        bool  // ADD THIS LINE
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		exitWithError(err)
	}

	ctx := context.Background()

	best, err := LoadBestPractices(cfg.Guidelines)
	if err != nil {
		exitWithError(err)
	}

	aiClient := NewAIClient(cfg.AIToken, cfg.AIEndpoint, cfg.AIModel, cfg.Temperature)

	diffs, err := GitLocalChanges(GitLocalOptions{
		RepoPath:        cfg.RepoPath,
		BaseRef:         cfg.TargetBranch,
		TargetBranch:    cfg.TargetBranch, // Pass this through
		IncludeUnstaged: true,
		Local:           cfg.Local,
	})
	if err != nil {
		exitWithError(err)
	}

	fmt.Printf("📊 Found %d changed files\n", len(diffs))

	// APPLY THE FILTER!
	diffs = FilterEligible(diffs, 0) // 0 = no limit

	fmt.Printf("🔍 Filtered to %d TypeScript/JavaScript files for review\n", len(diffs))

	// Initialize Tree-sitter parser
	parser := NewParser()
	if err := parser.Init(); err != nil {
		fmt.Printf("⚠️  Tree-sitter initialization failed: %v. Falling back to simple diff.\n", err)
		cfg.UseTreeSitter = false
	}
	defer parser.Close()

	comments := review(ctx, aiClient, parser, best, diffs, cfg.RepoPath, cfg.TargetBranch, cfg.UseTreeSitter)

	printLocal(comments)

	if countSeverity(comments, "critical") > 0 {
		os.Exit(1)
	}
}

type FileBatch struct {
	Files        []FileDiff
	TotalChanges int
}

// Update the review function signature and logic
func review(ctx context.Context, client *AIClient, parser *Parser, best string, diffs []FileDiff, repoPath, targetBranch string, useTreeSitter bool) []ReviewComment {
	// Create batches based on total changes
	batches := createBatches(diffs, 100) // 100 lines per batch

	fmt.Printf("📦 Created %d batch(es) for review\n\n", len(batches))

	var comments []ReviewComment
	for batchIdx, batch := range batches {
		fmt.Printf("🔄 Processing batch %d/%d (%d file(s), %d total changes)\n",
			batchIdx+1, len(batches), len(batch.Files), batch.TotalChanges)

		// Review the entire batch at once
		batchComments := reviewBatch(ctx, client, parser, best, batch, repoPath, targetBranch, useTreeSitter)
		comments = append(comments, batchComments...)

		fmt.Printf("  └─ Found %d issue(s) in this batch\n\n", len(batchComments))
	}

	return comments
}

func createBatches(diffs []FileDiff, maxChangesPerBatch int) []FileBatch {
	var batches []FileBatch
	var currentBatch FileBatch

	for _, diff := range diffs {
		fileChanges := diff.Additions + diff.Deletions

		// If this single file exceeds the limit, give it its own batch
		if fileChanges > maxChangesPerBatch {
			// Flush current batch if it has files
			if len(currentBatch.Files) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = FileBatch{}
			}

			// Add large file as its own batch
			batches = append(batches, FileBatch{
				Files:        []FileDiff{diff},
				TotalChanges: fileChanges,
			})
			continue
		}

		// If adding this file would exceed the limit, start a new batch
		if currentBatch.TotalChanges+fileChanges > maxChangesPerBatch && len(currentBatch.Files) > 0 {
			batches = append(batches, currentBatch)
			currentBatch = FileBatch{}
		}

		// Add file to current batch
		currentBatch.Files = append(currentBatch.Files, diff)
		currentBatch.TotalChanges += fileChanges
	}

	// Don't forget the last batch
	if len(currentBatch.Files) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

func reviewBatch(ctx context.Context, client *AIClient, parser *Parser, best string, batch FileBatch, repoPath, targetBranch string, useTreeSitter bool) []ReviewComment {
	// Enrich all files in the batch with context
	var enrichedDiffs []FileDiff
	var contexts []*CodeContext

	for _, diff := range batch.Files {
		fmt.Printf("  📄 %s (+%d -%d)", diff.NewPath, diff.Additions, diff.Deletions)

		var context *CodeContext
		var enrichedDiff FileDiff
		var err error

		if useTreeSitter && parser != nil {
			enrichedDiff, context, err = enrichDiffWithContext(repoPath, diff, targetBranch)
			if err != nil {
				fmt.Printf("  ⚠️  Failed to enrich context: %v\n", err)
				enrichedDiff = diff
				context = nil
			}
		} else {
			enrichedDiff = diff
			context = nil
		}

		enrichedDiffs = append(enrichedDiffs, enrichedDiff)
		contexts = append(contexts, context)
		fmt.Println()
	}

	// Send entire batch to AI in one request
	resp, err := client.ReviewBatch(ctx, best, enrichedDiffs, contexts)
	if err != nil {
		fmt.Printf("  ❌ Batch review failed: %v\n", err)
		return nil
	}

	return resp.Comments
}

func printLocal(comments []ReviewComment) {
	if len(comments) == 0 {
		fmt.Println("\n✅ All clear! No issues found.\n")
		return
	}

	// Group comments by file
	byFile := make(map[string][]ReviewComment)
	for _, c := range comments {
		byFile[c.FilePath] = append(byFile[c.FilePath], c)
	}

	// Sort files for consistent output
	var files []string
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	fmt.Println("\n" + strings.Repeat("═", 80))
	fmt.Println("📋 CODE REVIEW RESULTS")
	fmt.Println(strings.Repeat("═", 80) + "\n")

	totalIssues := 0
	for _, file := range files {
		fileComments := byFile[file]
		totalIssues += len(fileComments)

		// Print file header
		fmt.Printf("📄 %s\n", file)
		fmt.Println(strings.Repeat("─", 80))

		// Sort comments by line number
		sort.Slice(fileComments, func(i, j int) bool {
			return fileComments[i].Line < fileComments[j].Line
		})

		for _, c := range fileComments {
			emoji := getSeverityEmoji(c.Severity)
			color := getSeverityColor(c.Severity)

			fmt.Printf("  %s Line %d: %s%s%s\n",
				emoji,
				c.Line,
				color,
				strings.ToUpper(c.Severity),
				"\033[0m", // Reset color
			)

			// Word wrap the comment at 76 chars (80 - 4 for indent)
			wrappedComment := wordWrap(c.Comment, 76)
			for _, line := range strings.Split(wrappedComment, "\n") {
				fmt.Printf("    %s\n", line)
			}
			fmt.Println() // Empty line between issues
		}
	}

	fmt.Println(strings.Repeat("═", 80))
	fmt.Printf("Found %d issue(s) across %d file(s)\n", totalIssues, len(files))
	fmt.Println(strings.Repeat("═", 80) + "\n")
}

func getSeverityEmoji(severity string) string {
	severity = strings.ToLower(severity)
	switch {
	case strings.Contains(severity, "blocking"):
		return "🚨  "
	case strings.Contains(severity, "question"):
		return "❓  "
	case strings.Contains(severity, "issue"):
		return "⚠️  "
	case strings.Contains(severity, "suggestion"):
		return "💡  "
	default:
		return "ℹ️  "
	}
}

func getSeverityColor(severity string) string {
	severity = strings.ToLower(severity)
	switch {
	case strings.Contains(severity, "blocking"):
		return "\033[1;31m" // Bold Red
	case strings.Contains(severity, "question"):
		return "\033[1;33m" // Bold Yellow
	case strings.Contains(severity, "issue"):
		return "\033[1;33m" // Bold Yellow
	case strings.Contains(severity, "suggestion"):
		return "\033[1;36m" // Bold Cyan
	default:
		return "\033[0m" // Default
	}
}

func wordWrap(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	var currentLine []string
	currentLength := 0

	for _, word := range words {
		wordLen := len(word)
		// +1 for the space before the word
		if currentLength > 0 && currentLength+wordLen+1 > width {
			lines = append(lines, strings.Join(currentLine, " "))
			currentLine = []string{word}
			currentLength = wordLen
		} else {
			currentLine = append(currentLine, word)
			if currentLength > 0 {
				currentLength++ // space
			}
			currentLength += wordLen
		}
	}

	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, " "))
	}

	return strings.Join(lines, "\n")
}

func countSeverity(comments []ReviewComment, severity string) int {
	count := 0
	for _, c := range comments {
		if c.Severity == severity {
			count++
		}
	}
	return count
}

func loadConfig() (config, error) {
	env := func(fallback string, keys ...string) string {
		for _, key := range keys {
			if val := os.Getenv(key); val != "" {
				return val
			}
		}
		return fallback
	}

	fs := pflag.NewFlagSet("review", pflag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "AI code review CLI with Tree-sitter\n\nExamples:\n  go run ./cmd/review --ai-token $AI_TOKEN\n  go run ./cmd/review --ai-token $AI_TOKEN --target-branch main\n\nFlags:\n")
		fs.PrintDefaults()
	}
	aiToken := fs.String("ai-token", env("", "SCW_SECRET_KEY_AI_USER", "AI_TOKEN"), "Scaleway AI token")
	aiEndpoint := fs.String("ai-endpoint", env("https://api.scaleway.ai/3e211a1d-e19d-4e63-b47f-c88d70377aac/v1", "SCALEWAY_AI_ENDPOINT"), "Scaleway AI endpoint")
	aiModel := fs.String("ai-model", env("qwen3-235b-a22b-instruct-2507", "SCALEWAY_AI_MODEL"), "AI model name")
	temp := fs.Float64("temperature", envFloat("REVIEW_TEMPERATURE", 0.0), "Sampling temperature for the AI model (use 0 for consistent results)")
	guidelinesDir := fs.String("guidelines-dir", defaultGuidelinesDir(), "Guidelines directory")
	useTreeSitter := fs.Bool("tree-sitter", envBool("USE_TREE_SITTER", true), "Use Tree-sitter for enhanced context")

	repoPath := fs.String("path", ".", "Path to repository when running locally")
	targetBranch := fs.String("target-branch", env("HEAD", "TARGET_BRANCH"), "Base branch for local diffs")
	local := fs.Bool("local", envBool("LOCAL", false), "Compare local changes (staged + unstaged) to origin/target-branch")

	fs.AddGoFlagSet(flag.CommandLine)
	_ = fs.Parse(os.Args[1:])

	if *aiToken == "" {
		return config{}, errors.New("ai token is required")
	}

	cfg := config{
		AIToken:      *aiToken,
		AIEndpoint:   *aiEndpoint,
		AIModel:      *aiModel,
		Temperature:  *temp,
		Guidelines:   *guidelinesDir,
		RepoPath:     *repoPath,
		TargetBranch: *targetBranch,
		UseTreeSitter: *useTreeSitter,
		Local:        *local,
	}

	return cfg, nil
}

func envFloat(key string, fallback float64) float64 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func defaultGuidelinesDir() string {
	candidates := []string{
		"rules",
		filepath.Join(currentDir(), "rules"),
		"documentation/guidelines",
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return candidates[0]
}

func currentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
