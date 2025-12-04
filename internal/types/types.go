// Package types defines core data structures used across the code review tool.
package types

// FileDiff represents a single file's changes in a diff.
type FileDiff struct {
    OldPath   string
    NewPath   string
    Diff      string
    Additions int
    Deletions int
    Language  string
}

// ReviewComment represents a single code review comment from the AI.
type ReviewComment struct {
    FilePath string `json:"filePath"`
    Line     int    `json:"line"`
    Comment  string `json:"comment"`
    Severity string `json:"severity"`
}

// AIReviewResponse is the structured response from the AI review API.
type AIReviewResponse struct {
    Comments []ReviewComment `json:"comments"`
    Summary  string          `json:"summary"`
}

// CodeContext holds contextual information about changed code lines.
type CodeContext struct {
    ChangedLines []int          // Line numbers that were changed
    Surrounding  map[int]string // Line number -> surrounding context (5 lines before/after)
}

// FileBatch represents a batch of files to be reviewed together.
type FileBatch struct {
    Files        []FileDiff
    TotalChanges int
}
