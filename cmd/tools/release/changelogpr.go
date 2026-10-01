package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/releasenotes"
)

// promoted is the change `changelog promote` leaves in the worktree: the new
// release entry, and the fragments it collapsed and deleted.
type promoted struct {
	version   string
	entryPath string
	fragments []string
}

// readPromotedWorktree reads what promote left behind, and refuses anything
// else. The release-notes commit has to contain the entry and the deleted
// fragments and nothing besides, so this reads the worktree rather than
// trusting a caller to stage the right files.
func readPromotedWorktree(ctx context.Context) (promoted, error) {
	status, err := commandOutput(ctx, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return promoted{}, fmt.Errorf("read worktree: %w", err)
	}
	return parsePromotedStatus(status)
}

func parsePromotedStatus(status string) (promoted, error) {
	var result promoted
	var unexpected []string
	for line := range strings.SplitSeq(strings.TrimRight(status, "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		code, file := line[:2], line[3:]
		if strings.HasPrefix(file, `"`) {
			return promoted{}, fmt.Errorf("worktree holds a path git had to quote (%s); resolve it by hand", file)
		}
		switch {
		case code == "??" && path.Dir(file) == changelogDir && strings.HasSuffix(file, ".md"):
			if result.entryPath != "" {
				return promoted{}, fmt.Errorf("two new changelog entries (%s and %s); expected one", result.entryPath, file)
			}
			result.entryPath = file
			result.version = strings.TrimSuffix(path.Base(file), ".md")
		case (code == " D" || code == "D ") && path.Dir(file) == unreleasedDir():
			result.fragments = append(result.fragments, file)
		default:
			unexpected = append(unexpected, file)
		}
	}

	if len(unexpected) > 0 {
		return promoted{}, fmt.Errorf(
			"worktree holds changes that are not the promoted release notes: %s\ncommit or stash them, then run `mise run desktop:changelog:promote` again",
			strings.Join(unexpected, ", "))
	}
	if result.entryPath == "" {
		return promoted{}, errors.New("no promoted changelog entry in the worktree; run `mise run desktop:changelog:promote -- <stable|version>` first")
	}
	if len(result.fragments) == 0 {
		return promoted{}, fmt.Errorf("%s exists but no fragments were deleted; this is not a promotion", result.entryPath)
	}
	return result, nil
}

// validatePromoteSource requires a clean main before promotion, so that
// everything the worktree holds afterwards is the promotion itself. That is
// what lets `changelog pr` commit the release notes without being told which
// files they are.
func validatePromoteSource(ctx context.Context) error {
	state, err := loadReleaseSourceState(ctx)
	if err != nil {
		return err
	}
	if state.dirty {
		return errors.New("working tree is not clean: promote from a clean main, so the release-notes commit holds nothing else")
	}
	if state.branch != "main" {
		return fmt.Errorf("current branch is %q, want main", displayBranch(state.branch))
	}
	if state.head != state.originMain {
		return fmt.Errorf("HEAD %s does not equal origin/main %s; land or rebase your work first", state.head, state.originMain)
	}
	return nil
}

// openReleaseNotesPR commits the promoted entry on its own branch and opens the
// pull request that lands it.
//
// `release publish` refuses a stable version whose entry is not on main, and
// step 3 of a release requires a clean tree identical to origin/main — so the
// entry can never be written during the release run. This is that separate
// landing, done by one command rather than by hand, so the branch name, the
// commit subject and the PR title are the same every release.
func openReleaseNotesPR(ctx context.Context, dryRun bool) error {
	state, err := loadReleaseSourceState(ctx)
	if err != nil {
		return err
	}
	if state.branch != "main" {
		return fmt.Errorf("release notes branch from main, but the current branch is %q", displayBranch(state.branch))
	}
	if state.head != state.originMain {
		return fmt.Errorf("HEAD %s does not equal origin/main %s; land or rebase your work first", state.head, state.originMain)
	}

	found, err := readPromotedWorktree(ctx)
	if err != nil {
		return err
	}
	version, err := parsePublishVersion(found.version)
	if err != nil {
		return fmt.Errorf("promoted entry %s: %w", found.entryPath, err)
	}

	// go:embed reads the worktree at compile time, so `go run` sees the entry
	// that was just written and edited — the same read path the app uses.
	entries, err := releasenotes.Load()
	if err != nil {
		return fmt.Errorf("read changelog: %w", err)
	}
	entry, ok := entries.Find(found.version)
	if !ok {
		return fmt.Errorf("%s did not parse as %s", found.entryPath, found.version)
	}
	if err := validatePromotedEntry(entry, found.entryPath); err != nil {
		return err
	}

	branch := "chore/release-notes-" + found.version
	if err := requireBranchIsFree(ctx, branch); err != nil {
		return err
	}

	subject := fmt.Sprintf("chore(release): prepare %s release notes", found.version)
	if dryRun {
		fmt.Printf("would branch %s from main\n", branch)
		fmt.Printf("would commit %q\n", subject)
		fmt.Printf("  add     %s\n", found.entryPath)
		for _, fragment := range found.fragments {
			fmt.Printf("  delete  %s\n", fragment)
		}
		fmt.Printf("would push it and open a pull request titled %q\n", subject)
		return nil
	}

	if err := runCommand(ctx, "git", "switch", "--create", branch); err != nil {
		return err
	}
	if err := runCommand(ctx, "git", "add", "--", changelogDir); err != nil {
		return err
	}
	if err := runCommand(ctx, "git", "commit", "-m", subject, "-m", releaseNotesCommitBody(version, len(found.fragments))); err != nil {
		return err
	}
	if err := runCommand(ctx, "git", "push", "--set-upstream", "origin", branch); err != nil {
		return err
	}
	if err := runCommand(ctx, "gh", "pr", "create", "--title", subject, "--body", releaseNotesPRBody(entry)); err != nil {
		// The commit is pushed by this point, so the only step left is the one
		// that failed. Say that, or the next run hits requireBranchIsFree and
		// reads as though the whole command has to be undone.
		return fmt.Errorf(
			"%w\nthe release notes are committed and pushed to %s; open the pull request with:\n  gh pr create --title %q",
			err, branch, subject)
	}
	return nil
}

// validatePromotedEntry is the gate on an entry a person edited. Promotion
// writes an empty summary on purpose, and an empty one reaches the What's New
// toast and the channel manifest as nothing at all, so it has to be filled in
// before the entry can land.
func validatePromotedEntry(entry releasenotes.Entry, entryPath string) error {
	if entry.Summary == "" {
		return fmt.Errorf(
			"%s has no summary: write one sentence describing the release in its `summary:` header — it is what the What's New toast and the channel manifest show",
			entryPath)
	}
	if entry.Body == "" {
		return fmt.Errorf("%s has no body", entryPath)
	}
	return nil
}

func requireBranchIsFree(ctx context.Context, branch string) error {
	local, err := commandOutput(ctx, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("check branch %s: %w", branch, err)
	}
	if strings.TrimSpace(local) != "" {
		return fmt.Errorf(
			"branch %s already exists locally: if its commit is pushed, `gh pr create` is the only step left; otherwise delete the branch and promote again",
			branch)
	}
	remote, err := commandOutput(ctx, "git", "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("check branch %s on origin: %w", branch, err)
	}
	if strings.TrimSpace(remote) != "" {
		return fmt.Errorf("branch %s already exists on origin; its pull request is the one to land", branch)
	}
	return nil
}

// releaseNotesCommitBody keeps every interpolated value on a short line of its
// own. A version and a count spliced into a pre-wrapped sentence push it past
// the margin, and git does not reflow a commit body.
func releaseNotesCommitBody(version releaseVersion, fragments int) string {
	return fmt.Sprintf(
		"%s collects %d changelog fragments into one entry and deletes them.\n\n"+
			"The app embeds the notes in the binary, so `release publish` refuses a\nstable version that has no entry. This must land before the release runs.",
		version, fragments)
}

func releaseNotesPRBody(entry releasenotes.Entry) string {
	return fmt.Sprintf(
		"%s\n\nThis entry collects the changelog fragments that accumulated since the last stable release. The app embeds the notes in the binary, so `release publish` refuses a stable version that has no entry on main. This must land before the release runs.",
		entry.Summary)
}
