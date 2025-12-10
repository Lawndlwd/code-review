package types

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

type CodeContext struct {
	ChangedLines []int            // Line numbers that were changed
	Surrounding  map[int]string   // Line number -> surrounding context (5 lines before/after)
}

type FileBatch struct {
	Files        []FileDiff
	TotalChanges int
}
