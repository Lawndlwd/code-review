```
  ██████  ██████  ██       ██    ██  ██████  ███    ██
 ██       ██   ██ ██       ██    ██ ██    ██ ████   ██
 ██   ███ ██████  ██       ██    ██ ██    ██ ██ ██  ██
 ██    ██ ██   ██ ██       ██    ██ ██    ██ ██  ██ ██
  ██████  ██   ██ ███████  ███████  ██████  ██   ████
```

# Golum

**AI-powered code review tool** that analyzes TypeScript/JavaScript changes against your coding guidelines using Scaleway AI. Golum helps maintain code quality by automatically reviewing your code changes and providing actionable feedback based on your team's best practices.

## Quick Start

### Run Directly (No Installation)

```bash
go run github.com/lawndlwd/golum@main \
  --project-path ../project-name \
  --target-branch origin/main \
  --ai-token $AI_TOKEN \
  --rules-file ./rules/rules.md
```

### Install and Use

```bash
# Install
go install github.com/lawndlwd/golum@main

# Run
golum --project-path ../shire/ --target-branch origin/main --ai-token $AI_TOKEN --rules-file ./rules/rules.md
```

## Prerequisites

- Go 1.23+
- Git repository
- Scaleway AI API token (or compatible OpenAI API)
- Rules file (`.md` file or directory with `.md` files)

## Basic Usage

### Review Current Changes

```bash
go run github.com/lawndlwd/golum@main \
  --project-path ../project-name \
  --target-branch origin/main \
  --ai-token $AI_TOKEN \
  --rules-file ./rules/rules.md
```

### Review Against a Branch

```bash
go run github.com/lawndlwd/golum@main \
  --project-path ../project-name \
  --target-branch origin/main \
  --ai-token $AI_TOKEN \
  --rules-file ./rules/rules.md
```

### Review Local Changes vs Remote

Compare your local (staged + unstaged) changes against a remote branch:

```bash
go run github.com/lawndlwd/golum@main \
  --project-path ../project-name \
  --target-branch origin/main \
  --ai-token $AI_TOKEN \
  --rules-file ./rules/rules.md \
  --local
```

## Command-Line Options

### Required Options

| Option | Description | Example |
|--------|-------------|---------|
| `--ai-token` | AI API token (required) | `--ai-token $AI_TOKEN` |
| `--rules-file` | Path to rules file or directory (required) | `--rules-file ./rules/rules.md` |

### AI Configuration

| Option | Description | Default | Environment Variable |
|--------|-------------|----------|---------------------|
| `--ai-token` | Scaleway AI API token | - | `SCW_SECRET_KEY_AI_USER`, `AI_TOKEN` |
| `--ai-endpoint` | AI endpoint URL | `https://api.scaleway.ai/3e211a1d-e19d-4e63-b47f-c88d70377aac/v1` | `SCALEWAY_AI_ENDPOINT` |
| `--ai-model` | AI model name | `qwen3-235b-a22b-instruct-2507` | `SCALEWAY_AI_MODEL` |
| `--temperature` | Sampling temperature (0 for deterministic) | `0.0` | `REVIEW_TEMPERATURE` |

### Rules Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `--rules-file` | Path to `.md` file or directory with `.md` files | - |
| `--rules-dir` | Rules directory (ignored if `--rules-file` is set) | - |

### Git Configuration

| Option | Description | Default | Environment Variable |
|--------|-------------|----------|---------------------|
| `--project-path` | Path to repository | `.` | - |
| `--target-branch` | Base branch for comparison | `HEAD` | `TARGET_BRANCH` |
| `--local` | Compare local changes to origin/target-branch | `false` | `LOCAL` |

### Advanced Options

| Option | Description | Default | Environment Variable |
|--------|-------------|----------|---------------------|
| `--tree-sitter` | Use Tree-sitter for enhanced context | `true` | `USE_TREE_SITTER` |

## Examples

### Using Environment Variables

```bash
export AI_TOKEN="your-token-here"
export TARGET_BRANCH="origin/main"

go run github.com/lawndlwd/golum@main \
  --project-path ../project-name \
  --target-branch origin/main \
  --rules-file ./rules/rules.md
```

### Custom AI Endpoint (OpenAI)

```bash
go run github.com/lawndlwd/golum@main \
  --project-path ../project-name \
  --target-branch origin/main \
  --ai-token $OPENAI_API_KEY \
  --ai-endpoint https://api.openai.com/v1 \
  --ai-model gpt-4 \
  --rules-file ./rules/rules.md
```

### Multiple Rules Files

If you have a directory with multiple `.md` files:

```bash
go run github.com/lawndlwd/golum@main \
  --project-path ../project-name \
  --target-branch origin/main \
  --ai-token $AI_TOKEN \
  --rules-file ./rules/
```

All `.md` files in the directory will be loaded and combined.

### Review Specific Repository

```bash
go run github.com/lawndlwd/golum@main \
  --project-path /path/to/your/repo \
  --target-branch origin/main \
  --ai-token $AI_TOKEN \
  --rules-file ./rules/rules.md
```

## Rules File Format

Create a Markdown file (`.md`) with your coding guidelines:

```markdown
# React & TypeScript Guidelines

## Naming Conventions
- Components: PascalCase (`UserProfile`, `DataTable`)
- Files: PascalCase for components, camelCase for utilities
- Hooks: camelCase with `use` prefix (`useUserData`)

## Component Structure
- Always use functional components with hooks
- Prefer named exports over default exports
- Destructure props in function signature
```

The tool will:
- Load a single `.md` file if you specify a file path
- Load all `.md` files from a directory if you specify a directory
- Sort files alphabetically and combine them

See `rules/rules.md` for a complete example.

## Output

The tool shows color-coded review results:

- 🚨 **Blocking Issues**: Critical problems that must be fixed
- ⚠️ **Issues**: Problems that should be addressed
- ❓ **Questions**: Questions about the code
- 💡 **Suggestions**: Non-blocking suggestions

Example:

```
════════════════════════════════════════════════════════════════════════════════
📋 CODE REVIEW RESULTS
════════════════════════════════════════════════════════════════════════════════

📄 src/components/Button.tsx
────────────────────────────────────────────────────────────────────────────────
  ⚠️  Line 42: ISSUE
    Violates naming convention: Component should use PascalCase.
    Fix by renaming to UserButton.

════════════════════════════════════════════════════════════════════════════════
Found 1 issue(s) across 1 file(s)
════════════════════════════════════════════════════════════════════════════════
```

## Exit Codes

- `0`: No critical issues found
- `1`: Critical (blocking) issues found

## How It Works

1. Analyzes git diffs to find changed TypeScript/JavaScript files
2. Filters to `.ts`, `.tsx`, `.js`, `.jsx` files only
3. Extracts code context around changed lines (using Tree-sitter)
4. Groups files into batches for efficient processing
5. Sends batches to AI with your rules and code context
6. Displays formatted review comments

## Building from Source

```bash
git clone https://github.com/lawndlwd/golum.git
cd golum
go build -o golum ./cmd
```

## Project Structure

```
golum/
├── cmd/
│   └── main.go              # CLI entry point
├── internal/
│   ├── types/               # Shared types
│   ├── parser/              # Tree-sitter parser
│   ├── diff/                # Diff processing
│   ├── ai/                  # AI client and prompts
│   ├── bestpractices/       # Rules loader
│   ├── filter/              # File filtering
│   ├── git/                 # Git operations
│   ├── review/              # Review orchestration
│   └── output/              # Output formatting
└── rules/
    └── rules.md             # Example rules
```

## Next Steps / Future Improvements

- [ ] **MCP Integration for Enhanced Reviews**: Integrate with Model Context Protocol (MCP) to search information from a list of resources (documentation, codebase knowledge, API specs) for more accurate and context-aware code reviews
- [ ] **GitLab MR Integration**: Add support for reviewing GitLab MRs directly with `--gitlab-host`, `--gitlab-token`, and `--gitlab-project` flags to fetch and review MR diffs automatically
- [ ] **Incremental Reviews**: Support reviewing only new changes since last review to avoid re-reviewing unchanged code
- [ ] **Custom Severity Levels**: Allow users to define custom severity levels and their meanings in the rules file
- [ ] **Multi-language Support**: Extend beyond TypeScript/JavaScript to support Python, Go, Rust, and other languages with appropriate parsers
- [ ] **Review Templates**: Support for different review templates (strict, lenient, security-focused) that can be selected via flags
- [ ] **Review History**: Track review history and show what changed between reviews
- [ ] **Interactive Mode**: Add an interactive mode to approve/reject suggestions and generate a summary report
- [ ] **Webhook Support**: Add webhook support to automatically trigger reviews on MR/PR events
- [ ] **Parallel Processing**: Process multiple files in parallel for faster reviews on large codebases
- [ ] **Caching**: Cache parsed code context and AI responses to speed up repeated reviews
