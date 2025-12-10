package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lawndlwd/code-review/internal/types"
)

type LocalOptions struct {
	RepoPath        string
	BaseRef         string
	TargetBranch    string
	IncludeUnstaged bool
	Local           bool
}

func LocalChanges(opts LocalOptions) ([]types.FileDiff, error) {
	repo := filepath.Clean(opts.RepoPath)
	// Use target branch if specified, otherwise compare to HEAD
	var compareRef string
	if opts.TargetBranch != "" && opts.TargetBranch != "HEAD" {
		compareRef = opts.TargetBranch
	} else {
		compareRef = "HEAD"
	}
	files, err := changedFiles(repo, compareRef, opts.IncludeUnstaged, opts.TargetBranch, opts.Local)
	if err != nil {
		return nil, err
	}
	var diffs []types.FileDiff
	for _, file := range files {
		diffText, err := diffFile(repo, compareRef, file, opts.IncludeUnstaged, opts.TargetBranch, opts.Local)
		if err != nil || strings.TrimSpace(diffText) == "" {
			continue
		}
		additions := countPrefix(diffText, '+')
		deletions := countPrefix(diffText, '-')
		diffs = append(diffs, types.FileDiff{
			OldPath:   file,
			NewPath:   file,
			Diff:      diffText,
			Additions: additions,
			Deletions: deletions,
		})
	}
	return diffs, nil
}

func changedFiles(repo, compareRef string, includeUnstaged bool, targetBranch string, local bool) ([]string, error) {
	var stdout []byte
	var err error

	if local {
		// Compare working directory + staged changes to origin/targetBranch
		originBranch := targetBranch
		args := []string{"-C", repo, "diff", "--name-only", originBranch}
		stdout, err = exec.Command("git", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("git diff %s: %w", originBranch, err)
		}
	} else if targetBranch != "" && targetBranch != "HEAD" {
		// Find the merge base (where your branch diverged from target)
		mergeBaseCmd := exec.Command("git", "-C", repo, "merge-base", targetBranch, "HEAD")
		mergeBase, err := mergeBaseCmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git merge-base: %w", err)
		}
		baseCommit := strings.TrimSpace(string(mergeBase))

		// Now diff from that merge base to HEAD (only YOUR changes)
		args := []string{"-C", repo, "diff", "--name-only", baseCommit, "HEAD"}
		stdout, err = exec.Command("git", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("git diff %s HEAD: %w", baseCommit, err)
		}
	} else {
		args := []string{"-C", repo, "diff", "--name-only", compareRef}
		stdout, err = exec.Command("git", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("git diff --name-only: %w", err)
		}
	}

	entries := parseLines(stdout)
	if includeUnstaged {
		cachedArgs := []string{"-C", repo, "diff", "--cached", "--name-only"}
		cached, err := exec.Command("git", cachedArgs...).Output()
		if err == nil {
			entries = append(entries, parseLines(cached)...)
		}
	}
	seen := make(map[string]struct{})
	var unique []string
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		unique = append(unique, entry)
	}
	return unique, nil
}

func diffFile(repo, compareRef, path string, includeUnstaged bool, targetBranch string, local bool) (string, error) {
	var buf bytes.Buffer
	if includeUnstaged {
		args := []string{"-C", repo, "diff", "--cached", compareRef, "--", path}
		if out, err := exec.Command("git", args...).Output(); err == nil {
			buf.Write(out)
		}
	}

	var args []string
	if local {
		// Compare working directory to origin/targetBranch
		originBranch := targetBranch
		args = []string{"-C", repo, "diff", originBranch, "--", path}
	} else if targetBranch != "" && targetBranch != "HEAD" {
		// Find merge base
		mergeBaseCmd := exec.Command("git", "-C", repo, "merge-base", targetBranch, "HEAD")
		mergeBase, err := mergeBaseCmd.Output()
		if err != nil {
			return "", fmt.Errorf("git merge-base: %w", err)
		}
		baseCommit := strings.TrimSpace(string(mergeBase))

		// Diff from merge base to HEAD
		args = []string{"-C", repo, "diff", baseCommit, "HEAD", "--", path}
	} else {
		args = []string{"-C", repo, "diff", compareRef, "--", path}
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s: %w", path, err)
	}
	buf.Write(out)
	return buf.String(), nil
}

func parseLines(input []byte) []string {
	raw := strings.Split(string(input), "\n")
	var result []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func countPrefix(diff string, prefix rune) int {
	count := 0
	for _, line := range strings.Split(diff, "\n") {
		if len(line) > 0 && rune(line[0]) == prefix {
			count++
		}
	}
	return count
}
