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
// GitHub release for every program, and deploys the site. It waits for the
// run to finish.
//
// The desktop artifacts stay in R2 (decision 0003); the release attaches the
// CLI's archives only. It is idempotent: an existing tag at the release commit
// is reused and an existing release is left alone, so it can be re-run to
// recover a release whose GitHub step failed after the irreversible R2 upload.
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
	exists, err := gitHubReleaseExists(ctx, tag)
	if err != nil {
		return err
	}
	if exists {
		fmt.Printf("==> GitHub release %s already exists; leaving it unchanged\n", tag)
		return nil
	}
	return dispatchPublishWorkflow(ctx, tag)
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

const publishWorkflow = "publish.yml"

// dispatchPublishWorkflow starts the publish workflow for tag and waits for
// it. `gh workflow run` prints no run id, so the run is the newest dispatch of
// the workflow created after the call.
func dispatchPublishWorkflow(ctx context.Context, tag string) error {
	started := time.Now().UTC().Add(-time.Minute)
	fmt.Printf("==> dispatching %s for %s\n", publishWorkflow, tag)
	if err := runCommand(ctx, "gh", "workflow", "run", publishWorkflow, "--ref", "main", "--field", "tag="+tag); err != nil {
		return err
	}

	runID, err := awaitWorkflowRun(ctx, started)
	if err != nil {
		return err
	}
	fmt.Printf("==> watching run %s\n", runID)
	if err := runCommand(ctx, "gh", "run", "watch", runID, "--exit-status", "--interval", "30"); err != nil {
		return fmt.Errorf("%w\nthe desktop release is live; after fixing the run, finish with `go run ./cmd/tools/release github %s`",
			err, strings.TrimPrefix(tag, "v"))
	}
	return nil
}

type workflowRun struct {
	ID        int64     `json:"databaseId"`
	CreatedAt time.Time `json:"createdAt"`
}

func awaitWorkflowRun(ctx context.Context, after time.Time) (string, error) {
	for range 20 {
		output, err := commandOutput(ctx, "gh", "run", "list", "--workflow", publishWorkflow,
			"--event", "workflow_dispatch", "--limit", "5", "--json", "databaseId,createdAt")
		if err != nil {
			return "", err
		}
		if id, ok := newestRunAfter(output, after); ok {
			return id, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return "", fmt.Errorf("the %s run did not appear; find it with `gh run list --workflow %s`", publishWorkflow, publishWorkflow)
}

func newestRunAfter(output string, after time.Time) (string, bool) {
	var runs []workflowRun
	if err := json.Unmarshal([]byte(output), &runs); err != nil {
		return "", false
	}
	var newest *workflowRun
	for i := range runs {
		if runs[i].CreatedAt.Before(after) {
			continue
		}
		if newest == nil || runs[i].CreatedAt.After(newest.CreatedAt) {
			newest = &runs[i]
		}
	}
	if newest == nil {
		return "", false
	}
	return strconv.FormatInt(newest.ID, 10), true
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
