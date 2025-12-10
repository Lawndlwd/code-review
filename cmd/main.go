package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/lawndlwd/code-review/internal/ai"
	"github.com/lawndlwd/code-review/internal/bestpractices"
	"github.com/lawndlwd/code-review/internal/filter"
	"github.com/lawndlwd/code-review/internal/git"
	"github.com/lawndlwd/code-review/internal/output"
	"github.com/lawndlwd/code-review/internal/parser"
	"github.com/lawndlwd/code-review/internal/review"
	"github.com/spf13/pflag"
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

	// APPLY THE FILTER!
	diffs = filter.FilterEligible(diffs, 0) // 0 = no limit

	fmt.Printf("🔍 Filtered to %d TypeScript/JavaScript files for review\n", len(diffs))

	// Initialize Tree-sitter parser
	p := parser.NewParser()
	if err := p.Init(); err != nil {
		fmt.Printf("⚠️  Tree-sitter initialization failed: %v. Falling back to simple diff.\n", err)
		cfg.UseTreeSitter = false
	}
	defer p.Close()

	comments := review.Review(ctx, aiClient, p, best, diffs, cfg.RepoPath, cfg.TargetBranch, cfg.UseTreeSitter)

	output.PrintLocal(comments)

	if output.CountSeverity(comments, "critical") > 0 {
		os.Exit(1)
	}
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
		fmt.Fprintf(os.Stderr, "AI code review CLI with Tree-sitter\n\nExamples:\n  code-review --project-path ../project-name --target-branch origin/main --ai-token $AI_TOKEN --rules-file ./rules/rules.md\n  code-review --project-path ../project-name --target-branch origin/main --ai-token $AI_TOKEN --rules-file /path/to/rules\n\nFlags:\n")
		fs.PrintDefaults()
	}
	aiToken := fs.String("ai-token", env("", "SCW_SECRET_KEY_AI_USER", "AI_TOKEN"), "Scaleway AI token")
	aiEndpoint := fs.String("ai-endpoint", env("https://api.scaleway.ai/3e211a1d-e19d-4e63-b47f-c88d70377aac/v1", "SCALEWAY_AI_ENDPOINT"), "Scaleway AI endpoint")
	aiModel := fs.String("ai-model", env("qwen3-235b-a22b-instruct-2507", "SCALEWAY_AI_MODEL"), "AI model name")
	temp := fs.Float64("temperature", envFloat("REVIEW_TEMPERATURE", 0.0), "Sampling temperature for the AI model (use 0 for consistent results)")
	rulesFile := fs.String("rules-file", "", "Path to rules file (.md) or directory containing .md files (overrides --rules-dir)")
	rulesDir := fs.String("rules-dir", defaultRulesDir(), "Rules directory (ignored if --rules-file is set)")
	useTreeSitter := fs.Bool("tree-sitter", envBool("USE_TREE_SITTER", true), "Use Tree-sitter for enhanced context")

	repoPath := fs.String("project-path", ".", "Path to repository when running locally")
	targetBranch := fs.String("target-branch", env("HEAD", "TARGET_BRANCH"), "Base branch for local diffs")
	local := fs.Bool("local", envBool("LOCAL", false), "Compare local changes (staged + unstaged) to origin/target-branch")

	fs.AddGoFlagSet(flag.CommandLine)
	_ = fs.Parse(os.Args[1:])

	if *aiToken == "" {
		return config{}, errors.New("ai token is required")
	}

	// Use rules-file if provided, otherwise fall back to rules-dir
	rulesPath := *rulesDir
	if *rulesFile != "" {
		rulesPath = *rulesFile
	}

	// Validate that a rules path is provided
	if rulesPath == "" {
		return config{}, errors.New("rules file or directory is required. Use --rules-file to specify a path to a .md file or directory containing .md files")
	}

	cfg := config{
		AIToken:       *aiToken,
		AIEndpoint:    *aiEndpoint,
		AIModel:       *aiModel,
		Temperature:   *temp,
		Guidelines:    rulesPath,
		RepoPath:      *repoPath,
		TargetBranch:  *targetBranch,
		UseTreeSitter: *useTreeSitter,
		Local:         *local,
	}

	return cfg, nil
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

func defaultRulesDir() string {
	// When installed via go install, we can't rely on relative paths
	// Return empty string to force user to specify --rules-file
	// This prevents silent failures when rules aren't found
	return ""
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
