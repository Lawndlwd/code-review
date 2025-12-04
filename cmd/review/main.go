package main

import (
    "context"
    "errors"
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "strconv"

    "github.com/spf13/pflag"

    "github.com/levende/code-review/internal/ai"
    "github.com/levende/code-review/internal/loadRules"
    "github.com/levende/code-review/internal/diff"
    "github.com/levende/code-review/internal/filter"
    "github.com/levende/code-review/internal/git"
    "github.com/levende/code-review/internal/output"
    "github.com/levende/code-review/internal/parser"
    "github.com/levende/code-review/internal/types"
)

type config struct {
    AIToken       string
    AIEndpoint    string
    AIModel       string
    Temperature   float64
    Guidelines    string
    RepoPath      string
    TargetBranch  string
    UseTreeSitter bool
    Local         bool
}

func main() {
    cfg, err := loadConfig()
    if err != nil {
        exitWithError(err)
    }

    ctx := context.Background()

    best, err := bestpractices.LoadBestPractices(cfg.Guidelines)
    if err != nil {
        exitWithError(err)
    }

    aiClient := ai.NewClient(cfg.AIToken, cfg.AIEndpoint, cfg.AIModel, cfg.Temperature)

    diffs, err := git.LocalChanges(git.LocalOptions{
        RepoPath:        cfg.RepoPath,
        BaseRef:         cfg.TargetBranch,
        TargetBranch:    cfg.TargetBranch,
        IncludeUnstaged: true,
        Local:           cfg.Local,
    })
    if err != nil {
        exitWithError(err)
    }

    fmt.Printf("📊 Found %d changed files\n", len(diffs))

    diffs = filter.FilterEligible(diffs, 0)

    fmt.Printf("🔍 Filtered to %d TypeScript/JavaScript files for review\n", len(diffs))

    p := parser.NewParser()
    if err := p.Init(); err != nil {
        fmt.Printf("⚠️  Tree-sitter initialization failed: %v. Falling back to simple diff.\n", err)
        cfg.UseTreeSitter = false
    }
    defer p.Close()

    comments := review(ctx, aiClient, p, best, diffs, cfg.RepoPath, cfg.TargetBranch, cfg.UseTreeSitter)

    output.PrintLocal(comments)

    if output.CountSeverity(comments, "critical") > 0 {
        os.Exit(1)
    }
}

func review(ctx context.Context, client *ai.Client, p *parser.Parser, best string, diffs []types.FileDiff, repoPath, targetBranch string, useTreeSitter bool) []types.ReviewComment {
    batches := createBatches(diffs, 100)

    fmt.Printf("📦 Created %d batch(es) for review\n\n", len(batches))

    var comments []types.ReviewComment
    for batchIdx, batch := range batches {
        fmt.Printf("🔄 Processing batch %d/%d (%d file(s), %d total changes)\n",
            batchIdx+1, len(batches), len(batch.Files), batch.TotalChanges)

        batchComments := reviewBatch(ctx, client, p, best, batch, repoPath, targetBranch, useTreeSitter)
        comments = append(comments, batchComments...)

        fmt.Printf("  └─ Found %d issue(s) in this batch\n\n", len(batchComments))
    }

    return comments
}

func createBatches(diffs []types.FileDiff, maxChangesPerBatch int) []types.FileBatch {
    var batches []types.FileBatch
    var currentBatch types.FileBatch

    for _, diffItem := range diffs {
        fileChanges := diffItem.Additions + diffItem.Deletions

        if fileChanges > maxChangesPerBatch {
            if len(currentBatch.Files) > 0 {
                batches = append(batches, currentBatch)
                currentBatch = types.FileBatch{}
            }

            batches = append(batches, types.FileBatch{
                Files:        []types.FileDiff{diffItem},
                TotalChanges: fileChanges,
            })
            continue
        }

        if currentBatch.TotalChanges+fileChanges > maxChangesPerBatch && len(currentBatch.Files) > 0 {
            batches = append(batches, currentBatch)
            currentBatch = types.FileBatch{}
        }

        currentBatch.Files = append(currentBatch.Files, diffItem)
        currentBatch.TotalChanges += fileChanges
    }

    if len(currentBatch.Files) > 0 {
        batches = append(batches, currentBatch)
    }

    return batches
}

func reviewBatch(ctx context.Context, client *ai.Client, p *parser.Parser, best string, batch types.FileBatch, repoPath, targetBranch string, useTreeSitter bool) []types.ReviewComment {
    var enrichedDiffs []types.FileDiff
    var contexts []*types.CodeContext

    for _, d := range batch.Files {
        fmt.Printf("  📄 %s (+%d -%d)", d.NewPath, d.Additions, d.Deletions)

        var context *types.CodeContext
        var enrichedDiff types.FileDiff
        var err error

        if useTreeSitter && p != nil {
            enrichedDiff, context, err = diff.EnrichDiffWithContext(repoPath, d, targetBranch, p)
            if err != nil {
                fmt.Printf("  ⚠️  Failed to enrich context: %v\n", err)
                enrichedDiff = d
                context = nil
            }
        } else {
            enrichedDiff = d
            context = nil
        }

        enrichedDiffs = append(enrichedDiffs, enrichedDiff)
        contexts = append(contexts, context)
        fmt.Println()
    }

    resp, err := client.ReviewBatch(ctx, best, enrichedDiffs, contexts)
    if err != nil {
        fmt.Printf("  ❌ Batch review failed: %v\n", err)
        return nil
    }

    return resp.Comments
}

func loadConfig() (config, error) {
    env := func(fallback string, keys ...string) string {
        for _, key := range keys {
            if val := os.Getenv(key); val != "" {
                return val
            }
        }
        return fallback
    }

    fs := pflag.NewFlagSet("review", pflag.ExitOnError)
    fs.Usage = func() {
        fmt.Fprintf(os.Stderr, "AI code review CLI with Tree-sitter\n\nExamples:\n  go run ./cmd/review --ai-token $AI_TOKEN\n  go run ./cmd/review --ai-token $AI_TOKEN --target-branch main\n\nFlags:\n")
        fs.PrintDefaults()
    }

    aiToken := fs.String("ai-token", env("", "SCW_SECRET_KEY_AI_USER", "AI_TOKEN"), "Scaleway AI token")
    aiEndpoint := fs.String("ai-endpoint", env("https://api.scaleway.ai/3e211a1d-e19d-4e63-b47f-c88d70377aac/v1", "SCALEWAY_AI_ENDPOINT"), "Scaleway AI endpoint")
    aiModel := fs.String("ai-model", env("qwen3-235b-a22b-instruct-2507", "SCALEWAY_AI_MODEL"), "AI model name")
    temp := fs.Float64("temperature", envFloat("REVIEW_TEMPERATURE", 0.0), "Sampling temperature")
    guidelinesDir := fs.String("guidelines-dir", defaultGuidelinesDir(), "Guidelines directory")
    useTreeSitter := fs.Bool("tree-sitter", envBool("USE_TREE_SITTER", true), "Use Tree-sitter for enhanced context")
    repoPath := fs.String("path", ".", "Path to repository")
    targetBranch := fs.String("target-branch", env("HEAD", "TARGET_BRANCH"), "Base branch for diffs")
    local := fs.Bool("local", envBool("LOCAL", false), "Compare local changes to origin/target-branch")

    fs.AddGoFlagSet(flag.CommandLine)
    _ = fs.Parse(os.Args[1:])

    if *aiToken == "" {
        return config{}, errors.New("ai token is required")
    }

    return config{
        AIToken:       *aiToken,
        AIEndpoint:    *aiEndpoint,
        AIModel:       *aiModel,
        Temperature:   *temp,
        Guidelines:    *guidelinesDir,
        RepoPath:      *repoPath,
        TargetBranch:  *targetBranch,
        UseTreeSitter: *useTreeSitter,
        Local:         *local,
    }, nil
}

func envFloat(key string, fallback float64) float64 {
    if val := os.Getenv(key); val != "" {
        if parsed, err := strconv.ParseFloat(val, 64); err == nil {
            return parsed
        }
    }
    return fallback
}

func envBool(key string, fallback bool) bool {
    if val := os.Getenv(key); val != "" {
        if parsed, err := strconv.ParseBool(val); err == nil {
            return parsed
        }
    }
    return fallback
}

func defaultGuidelinesDir() string {
    candidates := []string{
        "rules",
        filepath.Join(currentDir(), "rules"),
        "documentation/guidelines",
    }
    for _, candidate := range candidates {
        if info, err := os.Stat(candidate); err == nil && info.IsDir() {
            return candidate
        }
    }
    return candidates[0]
}

func currentDir() string {
    dir, err := os.Getwd()
    if err != nil {
        return "."
    }
    return dir
}

func exitWithError(err error) {
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    os.Exit(1)
}
