// Package output handles formatting and printing review results.
package output

import (
    "fmt"
    "sort"
    "strings"

    "github.com/levende/code-review/internal/types"
)

// PrintLocal displays review comments in a human-readable format for local runs.
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
                "\033[0m",
            )

            wrappedComment := wordWrap(c.Comment, 76)
            for _, line := range strings.Split(wrappedComment, "\n") {
                fmt.Printf("    %s\n", line)
            }
            fmt.Println()
        }
    }

    fmt.Println(strings.Repeat("═", 80))
    fmt.Printf("Found %d issue(s) across %d file(s)\n", totalIssues, len(files))
    fmt.Println(strings.Repeat("═", 80) + "\n")
}

// CountSeverity counts comments with a specific severity level.
func CountSeverity(comments []types.ReviewComment, severity string) int {
    count := 0
    for _, c := range comments {
        if c.Severity == severity {
            count++
        }
    }
    return count
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
        if currentLength > 0 && currentLength+wordLen+1 > width {
            lines = append(lines, strings.Join(currentLine, " "))
            currentLine = []string{word}
            currentLength = wordLen
        } else {
            currentLine = append(currentLine, word)
            if currentLength > 0 {
                currentLength++
            }
            currentLength += wordLen
        }
    }

    if len(currentLine) > 0 {
        lines = append(lines, strings.Join(currentLine, " "))
    }

    return strings.Join(lines, "\n")
}
