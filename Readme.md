 ## 📦 Code‑Review CLI Tool  


A fast, AI‑assisted code‑review command line application written in Go.  
It analyses Git diffs (or any file list), enriches them with surrounding
source context, and sends batched prompts to an LLM endpoint.  
The tool prints colour‑coded comments with severity emojis, useful for CI
or local workflows.

---  

## Table of Contents  

1. [Overview](#overview)  
2. [Installation](#installation)  
3. [Quick Start](#quick-start)  
4. [CLI Options & Environment Variables](#cli-options--environment-variables)  
5. [Configuration File](#configuration-file)  
6. [How It Works – Architecture Overview](#how-it-works--architecture-overview)  
7. [Development Guide](#development-guide)  
   - [Running the Tests](#running-the-tests)  
   - [Adding a New Language Parser](#adding-a-new-language-parser)  
   - [Extending the AI Prompt](#extending-the-ai-prompt)  
8. [Contributing](#contributing)  
9. [License](#license)  

---  

## Overview  

The binary reads **Git local changes** (or a supplied list of `FileDiff`s),
extracts the changed lines together with a configurable amount of surrounding
source code, and sends the data in batches to an LLM via the `AIClient`.  
The LLM returns a structured JSON payload (`AIReviewResponse`) that is
rendered in the terminal with severity‑based colours and emojis.

Key concepts in the source:

| Symbol | Purpose |
|--------|---------|
| `FileDiff` | Holds path information, raw diff, and line counts. |
| `ReviewComment` | One comment returned by the LLM (file, line, text, severity). |
| `AIClient` | Thin HTTP wrapper around the configured LLM endpoint. |
| `Parser` | Uses **Tree‑Sitter** parsers (`js`, `ts`, `tsx`) to extract surrounding context. |
| `config` | Holds all runtime settings (token, endpoint, repo path, …). |
| `main` | Loads config, builds the `AIClient`, runs the review pipeline. |

---  

## Installation  

```bash
# Clone the repository
git clone https://github.com/yourorg/code-review.git
cd code-review

# Build the binary (requires Go ≥1.22)
go build -o code-review ./cmd.go
```

The executable is now `./code-review`.  
You can also install it with `go install` if you prefer:

```bash
go install github.com/yourorg/code-review/cmd.go@latest
```

---  

## Quick Start  

```bash
# Review the current branch against its base (default: origin/main)
./code-review -git -config=config.yaml
```

The command prints colour‑coded comments, e.g.:

```
⚠️  src/components/UserProfile.tsx:42  “Variable `isLoading` is never used.” (warning)
🛑  src/api/client.ts:108  “Hard‑coded endpoint, consider using config.” (blocker)
```

---  

## CLI Options & Environment Variables  

The tool can be configured **via flags** (preferred) **or** **environment
variables** (fallback). Flags are defined in `main()` and mapped directly to
the `config` struct.

| Flag / Env | Type | Description | Default |
|------------|------|-------------|---------|
| `-config` <br> `CODE_REVIEW_CONFIG` | `string` | Path to a YAML config file. | `./config.yaml` |
| `-aitoken` <br> `CODE_REVIEW_AI_TOKEN` | `string` | API key for the LLM service. | – |
| `-aiendpoint` <br> `CODE_REVIEW_AI_ENDPOINT` | `string` | Base URL of the LLM endpoint. | – |
| `-aimodel` <br> `CODE_REVIEW_AI_MODEL` | `string` | Model identifier (e.g. `gpt‑4o`). | `gpt-4o-mini` |
| `-temperature` <br> `CODE_REVIEW_TEMPERATURE` | `float64` | Sampling temperature for the LLM. | `0.2` |
| `-guidelines` <br> `CODE_REVIEW_GUIDELINES` | `string` | Directory containing best‑practice YAML files. | `./best‑practices` |
| `-repo` <br> `CODE_REVIEW_REPO_PATH` | `string` | Path to the Git repository. | current working directory |
| `-target` <br> `CODE_REVIEW_TARGET_BRANCH` | `string` | Branch to compare against (used with `-git`). | `main` |
| `-local` | `bool` | Skip Git integration; read diffs from stdin or a file list. | `false` |
| `-contextlines` | `int` | Number of surrounding lines to include for each change. | `5` |
| `-batchsize` | `int` | Maximum number of files per LLM request. | `10` |
| `-verbose` | `bool` | Print debug information (parser init, HTTP payloads). | `false` |
| `-printonly` | `bool` | Only print the generated comments, do not open an editor. | `false` |
| `-failbelow` | `string` | Exit with non‑zero status if a comment with equal or higher severity appears (`blocker`, `critical`, `major`, `minor`, `warning`). | – |

All flags are parsed in `main()` (see snippet below).

```go:/Users/levende/code-review/cmd.go#L722-770
func main() {
    // Load configuration (flags → env → defaults)
    cfg := loadConfig()

    // Initialise AI client
    client := NewAIClient(cfg.AIToken, cfg.AIEndpoint, cfg.AIModel, cfg.Temperature)

    // Run the review pipeline (Git or local mode)
    review(cfg, client)
}
```

---  

## Configuration File  

You can store the same options in a YAML file referenced by `-config`.  
Only fields that differ from defaults need to be present.

```yaml:/Users/levende/code-review/config.yaml#L1-15
aitoken: "<YOUR_API_KEY>"
aiendpoint: "https://api.openai.com/v1/chat/completions"
aimodel: "gpt-4o-mini"
temperature: 0.2
guidelines: "./best-practices"
repo: "."
target: "main"
useTreeSitter: true
local: false
contextlines: 6
batchsize: 12
verbose: false
```

The loader (`loadConfig`) merges flags, environment variables, and this file
into the internal `config` struct (see `loadConfig()` implementation).

---  

## How It Works – Architecture Overview  

```
+-------------------+          +--------------------+          +-------------------+
| Git / File List   |  ---->   |  Diff & Context    |  ---->   |  AIClient (LLM)   |
+-------------------+          +--------------------+          +-------------------+
        |                               |                               |
        |                               |                               |
        v                               v                               v
  GitLocalChanges                enrichDiffWithContext           ReviewBatch
        |                               |                               |
        |                               |                               |
        v                               v                               v
  []FileDiff  -> createBatches -> []FileBatch -> ReviewBatch -> AIReviewResponse
                                                                    |
                                                                    v
                                                            printLocal (terminal)
```

### 1. Gathering Changes  

* **Git mode** – `GitLocalChanges` uses `git diff` under the hood to obtain the
  diff of the current branch versus `TargetBranch`.  
* **Local mode** – `diffFile` parses a list of `FileDiff` objects supplied via
  stdin or a JSON file.

### 2. Adding Context  

`enrichDiffWithContext` calls `Parser.AnalyzeCodeContext` which selects the
appropriate Tree‑Sitter parser (`js`, `ts`, `tsx`) based on file extension
(`jsParser`, `tsParser`, `tsxParser`). It then:

* Parses the whole file (`getFileContent`).  
* Finds the changed lines (`parseChangedLines`).  
* Retrieves additional surrounding lines (`getSurroundingLines`).  

The result (`CodeContext`) contains `ChangedLines` and `Surrounding` slices
that are interpolated into the LLM prompt.

### 3. Building Prompt & Calling LLM  

`buildBatchPrompt` assembles a single prompt for a batch of files, inserting
the best‑practice guidelines loaded by `LoadBestPractices`.  
`AIClient.ReviewBatch` performs a POST request to the configured endpoint,
decodes the JSON response (`completionsResponse`), and extracts the first
content block (`FirstContent`).  

### 4. Parsing LLM Response  

`parseBatchResponse` unmarshals the JSON into `AIReviewResponse` which holds
a list of `ReviewComment` objects (file, line, comment, severity).  

### 5. Rendering Output  

`printLocal` iterates over the comments, applying colour codes (`getSeverityColor`)
and emojis (`getSeverityEmoji`). The helper `wordWrap` formats long messages.

---  

## Development Guide  

### Prerequisites  

* Go 1.22 or newer  
* `tree-sitter` libraries (automatically vendored via `go.mod`) – required for the parsers in `Parser`.  

### Project Layout  

```
/Users/levende/code-review/
├─ cmd.go                # Main entry point (CLI)
├─ best-practices/       # YAML guidelines
├─ go.mod / go.sum
└─ internal/ (optional) # Helper packages (future)
```

### Running the Tests  

The repository ships with unit tests for the parsing and prompt logic.  
Run them with:

```bash
go test ./... -v
```

### Adding a New Language Parser  

1. **Import the Tree‑Sitter grammar** in `cmd.go` (or a new file).  
2. Extend `Parser` struct with a new field, e.g. `pyParser *sitter.Parser`.  
3. Update `func (p *Parser) getParserForFile` to return the new parser for
   `"*.py"` extensions.  
4. Add tests exercising `parseChangedLines` on a Python file.  

### Extending the AI Prompt  

The prompt template lives in `buildBatchPrompt`. To add a new section:

* Modify the string concatenation inside `buildBatchPrompt` (lines 328‑409).  
* Update the corresponding unit test in `cmd_test.go`.  

### Building a Release Binary  

```bash
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/code-review-darwin-amd64 ./cmd.go
```

Replace `GOOS`/`GOARCH` with the target platform.

---  

## Contributing  

1. Fork the repository.  
2. Create a feature branch (`git checkout -b feat/awesome`).  
3. Write tests for any new behaviour.  
4. Ensure `go vet ./...` and `golint` (if enabled) pass.  
5. Open a Pull Request with a clear description and reference the issue.

Please keep the codebase aligned with the **sh**‑style guidelines (no `any`,
no inline styles, proper error handling, etc.).

---  

## License  

This project is licensed under the **MIT License** – see the `LICENSE` file for details.
