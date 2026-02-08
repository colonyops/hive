package git

import (
	"context"
	"fmt"
)

// DiffMode specifies the type of diff to retrieve.
type DiffMode int

const (
	// DiffUncommitted gets diffs for all uncommitted changes (working directory + staged).
	DiffUncommitted DiffMode = iota
	// DiffStaged gets diffs for only staged changes.
	DiffStaged
	// DiffBranch gets diffs between HEAD and a specified branch.
	DiffBranch
)

// DiffOptions specifies options for retrieving a git diff.
type DiffOptions struct {
	Mode       DiffMode
	BaseBranch string // Required for DiffBranch mode
}

// GetDiff retrieves a git diff based on the specified mode.
// Returns the unified diff as a string.
func (e *Executor) GetDiff(ctx context.Context, dir string, opts DiffOptions) (string, error) {
	var args []string

	switch opts.Mode {
	case DiffUncommitted:
		// Get all uncommitted changes (working directory + staged)
		args = []string{"diff", "HEAD"}

	case DiffStaged:
		// Get only staged changes
		args = []string{"diff", "--staged"}

	case DiffBranch:
		if opts.BaseBranch == "" {
			return "", fmt.Errorf("base branch required for DiffBranch mode")
		}
		// Get changes between base branch and HEAD
		// Using three-dot notation to compare against merge base
		args = []string{"diff", opts.BaseBranch + "...HEAD"}

	default:
		return "", fmt.Errorf("unknown diff mode: %d", opts.Mode)
	}

	out, err := e.exec.RunDir(ctx, dir, e.gitPath, args...)
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}

	return string(out), nil
}

// DescribeDiffMode returns a human-readable description of the diff mode.
func DescribeDiffMode(opts DiffOptions) string {
	switch opts.Mode {
	case DiffUncommitted:
		return "uncommitted changes"
	case DiffStaged:
		return "staged changes"
	case DiffBranch:
		return fmt.Sprintf("changes vs %s", opts.BaseBranch)
	default:
		return "unknown"
	}
}
