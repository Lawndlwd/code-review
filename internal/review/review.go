package review

import (
	"context"
	"fmt"

	"github.com/lawndlwd/code-review/internal/ai"
	diffpkg "github.com/lawndlwd/code-review/internal/diff"
	"github.com/lawndlwd/code-review/internal/parser"
	"github.com/lawndlwd/code-review/internal/types"
)

func Review(ctx context.Context, client *ai.Client, p *parser.Parser, best string, diffs []types.FileDiff, repoPath, targetBranch string, useTreeSitter bool) []types.ReviewComment {
	// Create batches based on total changes
	batches := createBatches(diffs, 100) // 100 lines per batch

	fmt.Printf("📦 Created %d batch(es) for review\n\n", len(batches))

	var comments []types.ReviewComment
	for batchIdx, batch := range batches {
		fmt.Printf("🔄 Processing batch %d/%d (%d file(s), %d total changes)\n",
			batchIdx+1, len(batches), len(batch.Files), batch.TotalChanges)

		// Review the entire batch at once
		batchComments := reviewBatch(ctx, client, p, best, batch, repoPath, targetBranch, useTreeSitter)
		comments = append(comments, batchComments...)

		fmt.Printf("  └─ Found %d issue(s) in this batch\n\n", len(batchComments))
	}

	return comments
}

func createBatches(diffs []types.FileDiff, maxChangesPerBatch int) []types.FileBatch {
	var batches []types.FileBatch
	var currentBatch types.FileBatch

	for _, diff := range diffs {
		fileChanges := diff.Additions + diff.Deletions

		// If this single file exceeds the limit, give it its own batch
		if fileChanges > maxChangesPerBatch {
			// Flush current batch if it has files
			if len(currentBatch.Files) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = types.FileBatch{}
			}

			// Add large file as its own batch
			batches = append(batches, types.FileBatch{
				Files:        []types.FileDiff{diff},
				TotalChanges: fileChanges,
			})
			continue
		}

		// If adding this file would exceed the limit, start a new batch
		if currentBatch.TotalChanges+fileChanges > maxChangesPerBatch && len(currentBatch.Files) > 0 {
			batches = append(batches, currentBatch)
			currentBatch = types.FileBatch{}
		}

		// Add file to current batch
		currentBatch.Files = append(currentBatch.Files, diff)
		currentBatch.TotalChanges += fileChanges
	}

	// Don't forget the last batch
	if len(currentBatch.Files) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

func reviewBatch(ctx context.Context, client *ai.Client, p *parser.Parser, best string, batch types.FileBatch, repoPath, targetBranch string, useTreeSitter bool) []types.ReviewComment {
	// Enrich all files in the batch with context
	var enrichedDiffs []types.FileDiff
	var contexts []*types.CodeContext

	for _, diff := range batch.Files {
		fmt.Printf("  📄 %s (+%d -%d)", diff.NewPath, diff.Additions, diff.Deletions)

		var context *types.CodeContext
		var enrichedDiff types.FileDiff
		var err error

		if useTreeSitter && p != nil {
			enrichedDiff, context, err = diffpkg.EnrichDiffWithContext(repoPath, diff, targetBranch, p)
			if err != nil {
				fmt.Printf("  ⚠️  Failed to enrich context: %v\n", err)
				enrichedDiff = diff
				context = nil
			}
		} else {
			enrichedDiff = diff
			context = nil
		}

		enrichedDiffs = append(enrichedDiffs, enrichedDiff)
		contexts = append(contexts, context)
		fmt.Println()
	}

	// Send entire batch to AI in one request
	resp, err := client.ReviewBatch(ctx, best, enrichedDiffs, contexts)
	if err != nil {
		fmt.Printf("  ❌ Batch review failed: %v\n", err)
		return nil
	}

	return resp.Comments
}
