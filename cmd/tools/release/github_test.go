package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTagTestRepo makes a repository with one commit and a bare origin, and
// makes it the working directory. The machine's git configuration is out of
// reach, so the result does not depend on it.
func newTagTestRepo(t *testing.T) (commit string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	tagTestGit(t, "init", "--quiet", "--bare", origin)
	tagTestGit(t, "init", "--quiet", "--initial-branch=main")
	tagTestGit(t, "remote", "add", "origin", origin)
	tagTestGit(t, "commit", "--quiet", "--allow-empty", "-m", "first")
	return tagTestGit(t, "rev-parse", "HEAD")
}

func tagTestGit(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s", strings.Join(args, " "), output)
	}
	return strings.TrimSpace(string(output))
}

// A release that failed after the tag push runs the GitHub step again, so
// both tag steps have to accept the tag they made the first time.
func TestEnsureTagsCreateThenReuseTheReleaseTag(t *testing.T) {
	commit := newTagTestRepo(t)

	for range 2 {
		if err := ensureLocalTag(t.Context(), "v0.60.0", commit); err != nil {
			t.Fatal(err)
		}
		if err := ensureOriginTag(t.Context(), "v0.60.0", commit); err != nil {
			t.Fatal(err)
		}
	}
	if got := tagTestGit(t, "rev-list", "-n", "1", "v0.60.0"); got != commit {
		t.Fatalf("local tag points at %s, want %s", got, commit)
	}
	if sha, err := originTagCommit(t.Context(), "v0.60.0"); err != nil || sha != commit {
		t.Fatalf("origin tag = %q, %v; want %s", sha, err, commit)
	}
}

func TestEnsureTagsRefuseATagAtAnotherCommit(t *testing.T) {
	commit := newTagTestRepo(t)
	tagTestGit(t, "tag", "v0.60.0", commit)
	tagTestGit(t, "push", "--quiet", "origin", "refs/tags/v0.60.0")
	tagTestGit(t, "commit", "--quiet", "--allow-empty", "-m", "second")
	head := tagTestGit(t, "rev-parse", "HEAD")

	err := ensureLocalTag(t.Context(), "v0.60.0", head)
	if err == nil || !strings.Contains(err.Error(), "not the release commit") {
		t.Fatalf("expected the local tag to be refused, got %v", err)
	}
	err = ensureOriginTag(t.Context(), "v0.60.0", head)
	if err == nil || !strings.Contains(err.Error(), "not the release commit") {
		t.Fatalf("expected the origin tag to be refused, got %v", err)
	}
}

func mustVersion(t *testing.T, value string) releaseVersion {
	t.Helper()
	version, err := parsePublishVersion(value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return version
}

func TestReleaseNotesHeader(t *testing.T) {
	header := releaseNotesHeader(mustVersion(t, "0.60.0"), "https://dl.hivedesktop.com")
	for _, want := range []string{
		"https://dl.hivedesktop.com/desktop/releases/0.60.0/",
		"https://dl.hivedesktop.com/desktop/releases/0.60.0/SHA256SUMS",
	} {
		if !strings.Contains(header, want) {
			t.Fatalf("header missing %q:\n%s", want, header)
		}
	}
}

// gh lists recent runs: other tags, and earlier runs for this one. The run to
// watch is the newest one for the tag created after the previous one.
func TestNewestRunFor(t *testing.T) {
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 1, hour, minute, 0, 0, time.UTC) }
	runs := []workflowRun{
		{ID: 1, Title: "Publish v0.60.0", CreatedAt: at(11, 0)},
		{ID: 2, Title: "Publish v0.61.0", CreatedAt: at(12, 0)},
		{ID: 3, Title: "Publish v0.60.0", CreatedAt: at(12, 9)},
		{ID: 4, Title: "fix(settings): a commit title from before run-name", CreatedAt: at(13, 0)},
	}

	run, ok := newestRunFor(runs, "v0.60.0", time.Time{})
	if !ok || run.ID != 3 {
		t.Fatalf("newestRunFor() = %+v, %t; want run 3", run, ok)
	}
	if _, ok := newestRunFor(runs, "v0.60.0", at(12, 9)); ok {
		t.Fatal("a run created at or before the previous one must not match")
	}
	if _, ok := newestRunFor(runs, "v0.62.0", time.Time{}); ok {
		t.Fatal("a run for another tag must not match")
	}
}

// GoReleaser creates the release before the cask publish and the site deploy,
// so an existing release does not mean the run finished, and a run that
// failed after creating it cannot be dispatched again.
func TestDecidePublishStep(t *testing.T) {
	tests := []struct {
		name          string
		run           workflowRun
		found         bool
		releaseExists bool
		want          publishStep
	}{
		{name: "no run and no release", want: stepDispatch},
		{name: "a run in progress", run: workflowRun{Status: "in_progress"}, found: true, want: stepWatch},
		{name: "a queued run", run: workflowRun{Status: "queued"}, found: true, releaseExists: true, want: stepWatch},
		{name: "a run that succeeded", run: workflowRun{Status: "completed", Conclusion: "success"}, found: true, releaseExists: true, want: stepDone},
		{name: "a run that failed before the release", run: workflowRun{Status: "completed", Conclusion: "failure"}, found: true, want: stepDispatch},
		{name: "a run that failed after the release", run: workflowRun{Status: "completed", Conclusion: "failure"}, found: true, releaseExists: true, want: stepRerun},
		{name: "a cancelled run after the release", run: workflowRun{Status: "completed", Conclusion: "cancelled"}, found: true, releaseExists: true, want: stepRerun},
		{name: "a release with no run to recover", releaseExists: true, want: stepDone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := decidePublishStep(test.run, test.found, test.releaseExists); got != test.want {
				t.Fatalf("decidePublishStep() = %d, want %d", got, test.want)
			}
		})
	}
}
