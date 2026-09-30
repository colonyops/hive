package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

type releaseSourceState struct {
	branch           string
	head             string
	originMain       string
	headOnOriginMain bool
	tagsAtHead       []string
	dirty            bool
}

func validatePrepareSource(ctx context.Context) error {
	state, err := loadReleaseSourceState(ctx)
	if err != nil {
		return err
	}
	return validatePrepareSourceState(state)
}

func validatePublishSource(ctx context.Context, version releaseVersion) error {
	state, err := loadReleaseSourceState(ctx)
	if err != nil {
		return err
	}
	return validatePublishSourceState(state, "desktop-v"+version.String())
}

func loadReleaseSourceState(ctx context.Context) (releaseSourceState, error) {
	if _, err := commandOutput(ctx, "git", "fetch", "--quiet", "--prune", "origin", "+refs/heads/main:refs/remotes/origin/main"); err != nil {
		return releaseSourceState{}, fmt.Errorf("refresh origin/main: %w", err)
	}
	status, err := commandOutput(ctx, "git", "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return releaseSourceState{}, fmt.Errorf("check release worktree: %w", err)
	}
	head, err := commandOutput(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return releaseSourceState{}, fmt.Errorf("resolve release commit: %w", err)
	}
	originMain, err := commandOutput(ctx, "git", "rev-parse", "--verify", "refs/remotes/origin/main")
	if err != nil {
		return releaseSourceState{}, fmt.Errorf("resolve origin/main (run git fetch origin main): %w", err)
	}
	tags, err := commandOutput(ctx, "git", "tag", "--points-at", "HEAD", "--list", "desktop-v*")
	if err != nil {
		return releaseSourceState{}, fmt.Errorf("list release tags at HEAD: %w", err)
	}
	headOnOriginMain := false
	command := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", "HEAD", "refs/remotes/origin/main")
	if err := command.Run(); err == nil {
		headOnOriginMain = true
	} else {
		exitErr := new(exec.ExitError)
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return releaseSourceState{}, fmt.Errorf("check release commit against origin/main: %w", err)
		}
	}

	branch := ""
	command = exec.CommandContext(ctx, "git", "symbolic-ref", "--quiet", "--short", "HEAD")
	output, err := command.Output()
	if err == nil {
		branch = strings.TrimSpace(string(output))
	} else {
		exitErr := new(exec.ExitError)
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return releaseSourceState{}, fmt.Errorf("resolve current branch: %w", err)
		}
	}

	return releaseSourceState{
		branch:           branch,
		head:             strings.TrimSpace(head),
		originMain:       strings.TrimSpace(originMain),
		headOnOriginMain: headOnOriginMain,
		tagsAtHead:       strings.Fields(tags),
		dirty:            strings.TrimSpace(status) != "",
	}, nil
}

func validatePrepareSourceState(state releaseSourceState) error {
	if state.dirty {
		return errors.New("prepare release: working tree is not clean")
	}
	if state.branch != "main" {
		return fmt.Errorf("prepare release: current branch is %q, want main", displayBranch(state.branch))
	}
	if state.head != state.originMain {
		return fmt.Errorf("prepare release: HEAD %s does not equal origin/main %s", state.head, state.originMain)
	}
	return nil
}

func validatePublishSourceState(state releaseSourceState, expectedTag string) error {
	if state.dirty {
		return errors.New("publish release: working tree is not clean")
	}
	switch {
	case state.branch == "main":
		if state.head != state.originMain {
			return fmt.Errorf("publish release: HEAD %s does not equal origin/main %s", state.head, state.originMain)
		}
		return nil
	case state.branch == "" && !slices.Contains(state.tagsAtHead, expectedTag):
		return fmt.Errorf("publish release: detached HEAD is not tagged %s", expectedTag)
	case state.branch == "" && !state.headOnOriginMain:
		return fmt.Errorf("publish release: tagged commit %s is not on origin/main", state.head)
	case state.branch == "":
		return nil
	default:
		return fmt.Errorf("publish release: current branch is %q, want main or detached HEAD at %s", state.branch, expectedTag)
	}
}

func displayBranch(branch string) string {
	if branch == "" {
		return "detached HEAD"
	}
	return branch
}
