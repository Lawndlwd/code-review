package diff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/lawndlwd/code-review/internal/parser"
	"github.com/lawndlwd/code-review/internal/types"
)

func ParseChangedLines(diff string) []int {
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

func EnrichDiffWithContext(repoPath string, diff types.FileDiff, targetBranch string, p *parser.Parser) (types.FileDiff, *types.CodeContext, error) {
	// Parse changed lines from diff
	changedLines := ParseChangedLines(diff.Diff)

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

	if p == nil {
		// If parser is nil, create a simple context without tree-sitter
		ctx := &types.CodeContext{
			ChangedLines: changedLines,
			Surrounding:  make(map[int]string),
		}
		return diff, ctx, nil
	}

	ctx := p.AnalyzeCodeContext(currentContent, changedLines, diff.NewPath)
	return diff, ctx, nil
}
