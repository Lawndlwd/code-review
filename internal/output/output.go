package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lawndlwd/code-review/internal/types"
)

func PrintLocal(comments []types.ReviewComment) {
	if len(comments) == 0 {
		fmt.Println("\n✅ All clear! No issues found.\n")
		return
	}

	// Group comments by file
	byFile := make(map[string][]types.ReviewComment)
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

			// Display severity and line number
			fmt.Printf("  %s Line %d: %s%s%s\n",
				emoji,
				c.Line,
				color,
				c.Severity,
				"\033[0m", // Reset color
			)

			// Word wrap the comment at 76 chars (80 - 4 for indent)
			// The comment already contains the severity prefix, so we display it as-is
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

func CountSeverity(comments []types.ReviewComment, severity string) int {
	count := 0
	for _, c := range comments {
		if c.Severity == severity {
			count++
		}
	}
	return count
}
