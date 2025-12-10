package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/lawndlwd/code-review/internal/types"
)

var fencedJSON = regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)\\s*```")

func BuildBatchPrompt(bestPractices string, files []types.FileDiff, contexts []*types.CodeContext) string {
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
	b.WriteString("   - **severity**: One of: \"suggestion(blocking)\", \"suggestion(non-blocking)\", \"issue\"\n")
	b.WriteString("   - **comment**: Write a humanized, conversational comment starting with the severity prefix. Examples:\n")
	b.WriteString("     * \"suggestion(blocking): Can you make a unit test here?\"\n")
	b.WriteString("     * \"suggestion(non-blocking): Can you make a unit test here?\"\n")
	b.WriteString("     * \"issue: Wait for production availability before deploying this feature\"\n")
	b.WriteString("     Write naturally and conversationally, as if you're a colleague reviewing the code.\n\n")

	b.WriteString("## Response Format - MANDATORY\n\n")
	b.WriteString("You MUST respond with ONLY valid JSON in this EXACT format (no additional text before or after):\n\n")
	b.WriteString("```json\n{\n  \"comments\": [\n    {\n      \"filePath\": \"exact/file/path.ts\",\n      \"line\": 42,\n      \"severity\": \"issue\",\n      \"comment\": \"issue: Wait for production availability before deploying this feature\"\n    },\n    {\n      \"filePath\": \"exact/file/path.ts\",\n      \"line\": 50,\n      \"severity\": \"suggestion(blocking)\",\n      \"comment\": \"suggestion(blocking): Can you make a unit test here?\"\n    }\n  ],\n  \"summary\": \"Found N violations across M files. Main issues: ...\"\n}\n```\n\n")

	b.WriteString("IMPORTANT:\n")
	b.WriteString("- If NO violations found, return: {\"comments\": [], \"summary\": \"No violations found\"}\n")
	b.WriteString("- Review files in order from File 1 to File N\n")
	b.WriteString("- Always use the same severity for the same type of violation\n")
	b.WriteString("- Always phrase comments the same way for identical violations\n")
	b.WriteString("- Write comments in a natural, humanized way - be conversational and friendly\n")
	b.WriteString("- The comment should start with the severity prefix (e.g., \"suggestion(blocking):\", \"issue:\")\n")

	return b.String()
}

func ParseBatchResponse(raw string) types.AIReviewResponse {
	jsonPayload := raw
	if matches := fencedJSON.FindStringSubmatch(raw); len(matches) == 2 {
		jsonPayload = matches[1]
	}

	var parsed types.AIReviewResponse
	if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
		return types.AIReviewResponse{
			Comments: nil,
			Summary:  "Failed to parse AI response",
		}
	}

	return parsed
}
