package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// publishGitHubRelease records a published version on GitHub and ships the
// CLI. It tags the release commit v<version>, pushes the tag, and dispatches
// the publish workflow, which builds the CLI with GoReleaser, creates the one
// GitHub release for every program, publishes the Homebrew cask, and deploys
// the site. It waits for the run to finish.
//
// The desktop artifacts stay in R2 (decision 0003); the release attaches the
// CLI's archives only. It can be re-run to recover a release whose GitHub
// step failed after the irreversible R2 upload: an existing tag at the
// release commit is reused, a run still in progress is watched, a run that
// succeeded is reported, and a run that failed after GoReleaser created the
// release is pointed at `gh run rerun --failed`, because a second run would
// collide with the release's assets.
//
// The caller must have already validated that HEAD is the release commit
// (validatePublishSource); the tag is created there.
func publishGitHubRelease(ctx context.Context, version releaseVersion) error {
	tag := version.tag()
	commit, err := gitHead(ctx)
	if err != nil {
		return err
	}
	if err := ensureLocalTag(ctx, tag, commit); err != nil {
		return err
	}
	if err := ensureOriginTag(ctx, tag, commit); err != nil {
		return err
	}

	runs, err := listPublishRuns(ctx)
	if err != nil {
		return err
	}
	run, found := newestRunFor(runs, tag, time.Time{})
	exists, err := gitHubReleaseExists(ctx, tag)
	if err != nil {
		return err
	}
	switch decidePublishStep(run, found, exists) {
	case stepWatch:
		fmt.Printf("==> %s run %s for %s is %s\n", publishWorkflow, run.id(), tag, run.Status)
		return watchWorkflowRun(ctx, run.id(), tag)
	case stepDone:
		if found {
			fmt.Printf("==> %s run %s already published %s\n", publishWorkflow, run.id(), tag)
		} else {
			fmt.Printf("==> GitHub release %s already exists; leaving it unchanged\n", tag)
		}
		return nil
	case stepRerun:
		return fmt.Errorf(
			"%s run %s for %s ended with %s after it created the GitHub release; fix the cause, then re-run its failed jobs with `gh run rerun %s --failed`",
			publishWorkflow, run.id(), tag, run.Conclusion, run.id())
	default:
		return dispatchPublishWorkflow(ctx, tag, run.CreatedAt)
	}
}

func ensureLocalTag(ctx context.Context, tag, commit string) error {
	exists, err := localTagExists(ctx, tag)
	if err != nil {
		return err
	}
	if exists {
		at, err := commandOutput(ctx, "git", "rev-list", "-n", "1", tag)
		if err != nil {
			return err
		}
		if strings.TrimSpace(at) != commit {
			return fmt.Errorf("local tag %s points at %s, not the release commit %s", tag, strings.TrimSpace(at), commit)
		}
		return nil
	}
	fmt.Printf("==> tagging %s\n", tag)
	return runCommand(ctx, "git", "tag", tag, commit)
}

func ensureOriginTag(ctx context.Context, tag, commit string) error {
	sha, err := originTagCommit(ctx, tag)
	if err != nil {
		return err
	}
	if sha != "" {
		if sha != commit {
			return fmt.Errorf("origin tag %s points at %s, not the release commit %s", tag, sha, commit)
		}
		fmt.Printf("==> origin already has tag %s\n", tag)
		return nil
	}
	fmt.Printf("==> pushing tag %s to origin\n", tag)
	return runCommand(ctx, "git", "push", "origin", "refs/tags/"+tag)
}

// originTagCommit returns the commit a tag points at on origin, or "" when the
// tag is absent. Release tags are lightweight, so ls-remote reports the
// commit directly rather than a tag object.
func originTagCommit(ctx context.Context, tag string) (string, error) {
	output, err := commandOutput(ctx, "git", "ls-remote", "--tags", "origin", "refs/tags/"+tag)
	if err != nil {
		return "", fmt.Errorf("check origin tag %s: %w", tag, err)
	}
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

func gitHubReleaseExists(ctx context.Context, tag string) (bool, error) {
	output, err := exec.CommandContext(ctx, "gh", "release", "view", tag, "--json", "tagName").CombinedOutput()
	if err == nil {
		return true, nil
	}
	// gh returns a non-zero exit both when the release is missing and on real
	// failures (auth, network), so the message is the only signal.
	if strings.Contains(string(output), "release not found") {
		return false, nil
	}
	exitErr := new(exec.ExitError)
	if errors.As(err, &exitErr) {
		return false, fmt.Errorf("gh release view %s: %s", tag, strings.TrimSpace(string(output)))
	}
	return false, fmt.Errorf("gh release view %s: %w", tag, err)
}

const (
	publishWorkflow = "publish.yml"
	// publishRunTitle is the run-name publish.yml sets, which is how a run is
	// found by its tag.
	publishRunTitle = "Publish "
)

type workflowRun struct {
	ID         int64     `json:"databaseId"`
	CreatedAt  time.Time `json:"createdAt"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Title      string    `json:"displayTitle"`
}

func (r workflowRun) id() string { return strconv.FormatInt(r.ID, 10) }

type publishStep int

const (
	stepDispatch publishStep = iota
	stepWatch
	stepDone
	stepRerun
)

// decidePublishStep picks what the GitHub step still has to do for a tag,
// from the newest publish run for it (if any) and whether the GitHub release
// exists. GoReleaser creates the release part-way through a run, before the
// Homebrew cask and the site deploy, so a release that exists does not mean
// the run finished, and a failed run that created the release cannot be
// dispatched again.
func decidePublishStep(run workflowRun, found, releaseExists bool) publishStep {
	switch {
	case found && run.Status != "completed":
		return stepWatch
	case found && run.Conclusion == "success":
		return stepDone
	case found && releaseExists:
		return stepRerun
	case releaseExists:
		return stepDone
	default:
		return stepDispatch
	}
}

// dispatchPublishWorkflow starts the publish workflow for tag and waits for
// it. previous is when the newest earlier run for the tag was created, or
// zero, so the run to watch is the one created after it.
func dispatchPublishWorkflow(ctx context.Context, tag string, previous time.Time) error {
	fmt.Printf("==> dispatching %s for %s\n", publishWorkflow, tag)
	if err := runCommand(ctx, "gh", "workflow", "run", publishWorkflow, "--ref", "main", "--field", "tag="+tag); err != nil {
		return err
	}
	run, err := awaitPublishRun(ctx, tag, previous)
	if err != nil {
		return err
	}
	return watchWorkflowRun(ctx, run.id(), tag)
}

func listPublishRuns(ctx context.Context) ([]workflowRun, error) {
	output, err := commandOutput(ctx, "gh", "run", "list", "--workflow", publishWorkflow,
		"--event", "workflow_dispatch", "--limit", "20", "--json", "databaseId,createdAt,status,conclusion,displayTitle")
	if err != nil {
		return nil, err
	}
	var runs []workflowRun
	if err := json.Unmarshal([]byte(output), &runs); err != nil {
		return nil, fmt.Errorf("parse gh run list: %w", err)
	}
	return runs, nil
}

// awaitPublishRun polls for the run a dispatch created. `gh workflow run`
// prints no run id, and GitHub lists the run a few seconds after the call.
func awaitPublishRun(ctx context.Context, tag string, previous time.Time) (workflowRun, error) {
	// A minute covers a slow API without hiding a dispatch that never ran.
	const attempts, interval = 20, 3 * time.Second
	for range attempts {
		runs, err := listPublishRuns(ctx)
		if err != nil {
			return workflowRun{}, err
		}
		if run, ok := newestRunFor(runs, tag, previous); ok {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return workflowRun{}, ctx.Err()
		case <-time.After(interval):
		}
	}
	return workflowRun{}, fmt.Errorf(
		"the %s run for %s did not appear; find it with `gh run list --workflow %s`, then run `go run ./cmd/tools/release github %s` to watch it",
		publishWorkflow, tag, publishWorkflow, strings.TrimPrefix(tag, "v"))
}

// newestRunFor returns the newest run for tag created after `after`.
func newestRunFor(runs []workflowRun, tag string, after time.Time) (workflowRun, bool) {
	var newest *workflowRun
	for i := range runs {
		run := &runs[i]
		if run.Title != publishRunTitle+tag || !run.CreatedAt.After(after) {
			continue
		}
		if newest == nil || run.CreatedAt.After(newest.CreatedAt) {
			newest = run
		}
	}
	if newest == nil {
		return workflowRun{}, false
	}
	return *newest, true
}

func watchWorkflowRun(ctx context.Context, runID, tag string) error {
	fmt.Printf("==> watching run %s\n", runID)
	// A run takes minutes; polling every 30 seconds stays well inside gh's
	// API rate limit and delays the result by less than a build step.
	if err := runCommand(ctx, "gh", "run", "watch", runID, "--exit-status", "--interval", "30"); err != nil {
		return fmt.Errorf(
			"%w\nthe desktop release is live; fix the cause, then re-run the failed jobs with `gh run rerun %s --failed`, or run `go run ./cmd/tools/release github %s` to watch the run or dispatch a new one",
			err, runID, strings.TrimPrefix(tag, "v"))
	}
	return nil
}

// releaseNotesHeader opens the GitHub release body. It states plainly that the
// desktop downloads come from R2, not this release's assets.
func releaseNotesHeader(version releaseVersion, downloadBase string) string {
	prefix := fmt.Sprintf("%s/desktop/releases/%s", downloadBase, version)
	return fmt.Sprintf(
		"The hive CLI archives are attached to this release. Hive Desktop downloads are served from Cloudflare R2, not GitHub (decision 0003).\n\n"+
			"- Desktop artifacts: %s/\n"+
			"- Desktop checksums: %s/SHA256SUMS\n",
		prefix, prefix,
	)
}
