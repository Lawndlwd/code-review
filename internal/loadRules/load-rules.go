package loadRules

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"
)

// LoadBestPractices loads all markdown files from a directory and concatenates them.
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
