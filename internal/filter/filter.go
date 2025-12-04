// Package filter provides file filtering logic for code review eligibility.
package filter

import (
    "strings"

    "github.com/levende/code-review/internal/types"
)

// FilterEligible filters files that should be reviewed.
func FilterEligible(files []types.FileDiff, limit int) []types.FileDiff {
    var result []types.FileDiff

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
