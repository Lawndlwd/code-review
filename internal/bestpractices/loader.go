package bestpractices

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadBestPractices(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}

	var files []string

	if info.IsDir() {
		// Load all .md files from directory
		pattern := filepath.Join(path, "*.md")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return "", fmt.Errorf("glob markdown: %w", err)
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("no markdown files found in directory %s", path)
		}
		files = matches
	} else {
		// Single file
		if !strings.HasSuffix(path, ".md") {
			return "", fmt.Errorf("rules file must be a .md file, got: %s", path)
		}
		files = []string{path}
	}

	sort.Strings(files)

	var builder strings.Builder

	for _, file := range files {
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
		return "", fmt.Errorf("no markdown guidelines found in %s", path)
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
