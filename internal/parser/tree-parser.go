// Package parser provides Tree-sitter based code parsing and context extraction.
package parser

import (
    "fmt"
    "strings"

    tree_sitter "github.com/tree-sitter/go-tree-sitter"
    tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"

    "github.com/levende/code-review/internal/types"
)

// Parser wraps Tree-sitter parsers for different languages.
type Parser struct {
    jsParser  *tree_sitter.Parser
    tsParser  *tree_sitter.Parser
    tsxParser *tree_sitter.Parser
}

// NewParser creates a new Parser instance.
func NewParser() *Parser {
    return &Parser{
        jsParser:  tree_sitter.NewParser(),
        tsParser:  tree_sitter.NewParser(),
        tsxParser: tree_sitter.NewParser(),
    }
}

// Init initializes all parsers with their respective language grammars.
func (p *Parser) Init() error {
    lang := tree_sitter.NewLanguage(tree_sitter_javascript.Language())

    p.jsParser.SetLanguage(lang)
    p.tsParser.SetLanguage(lang)
    p.tsxParser.SetLanguage(lang)

    return nil
}

// Close releases parser resources.
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

// AnalyzeCodeContext extracts contextual information around changed lines.
func (p *Parser) AnalyzeCodeContext(fileContent string, changedLines []int, filename string) *types.CodeContext {
    context := &types.CodeContext{
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
