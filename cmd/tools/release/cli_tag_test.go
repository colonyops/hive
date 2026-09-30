package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCLIVersion(t *testing.T) {
	tests := []struct {
		tag  string
		want cliVersion
	}{
		{tag: "v0.59.0", want: cliVersion{minor: 59}},
		{tag: "v1.2.3", want: cliVersion{major: 1, minor: 2, patch: 3}},
		{tag: "v1.2.3-rc1", want: cliVersion{major: 1, minor: 2, patch: 3}},
		{tag: "v1.2.3+build.5", want: cliVersion{major: 1, minor: 2, patch: 3}},
		{tag: "v10.20.30", want: cliVersion{major: 10, minor: 20, patch: 30}},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got, err := parseCLIVersion(tt.tag)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	for _, tag := range []string{"", "1.2.3", "v1.2", "v1.2.3.4", "v1.x.3", "v-1.2.3", "desktop/v1.2.3", "v2-experiment"} {
		t.Run("rejects "+tag, func(t *testing.T) {
			_, err := parseCLIVersion(tag)
			assert.Error(t, err)
		})
	}
}

func TestCLIVersionBump(t *testing.T) {
	version := cliVersion{major: 1, minor: 2, patch: 3}

	assert.Equal(t, "v1.2.4", version.bump(cliBumpPatch).String())
	assert.Equal(t, "v1.3.0", version.bump(cliBumpMinor).String())
	assert.Equal(t, "v2.0.0", version.bump(cliBumpMajor).String())
}

func TestParseCLIBumpLevel(t *testing.T) {
	for _, value := range []string{"patch", "minor", "major"} {
		level, err := parseCLIBumpLevel(value)
		require.NoError(t, err)
		assert.Equal(t, cliBumpLevel(value), level)
	}

	_, err := parseCLIBumpLevel("huge")
	assert.Error(t, err)
}

func TestPlanCLITag(t *testing.T) {
	tests := []struct {
		name    string
		state   cliTagState
		level   cliBumpLevel
		want    cliTagPlan
		wantErr error
	}{
		{
			name:  "bumps the nearest tag",
			state: cliTagState{nearest: "v0.59.0", nearestParent: "v0.59.0"},
			level: cliBumpPatch,
			want:  cliTagPlan{current: "v0.59.1", previous: "v0.59.0"},
		},
		{
			name:  "a minor bump resets the patch",
			state: cliTagState{nearest: "v0.59.7"},
			level: cliBumpMinor,
			want:  cliTagPlan{current: "v0.60.0", previous: "v0.59.7"},
		},
		{
			name:  "a tag on HEAD is used again and the level is ignored",
			state: cliTagState{headTags: []string{"v0.59.1"}, nearest: "v0.59.1", nearestParent: "v0.59.0"},
			level: cliBumpMajor,
			want:  cliTagPlan{current: "v0.59.1", previous: "v0.59.0", reuse: true},
		},
		{
			name:  "the newest of two tags on HEAD wins",
			state: cliTagState{headTags: []string{"v0.9.0", "v0.10.0"}, nearestParent: "v0.8.0"},
			level: cliBumpPatch,
			want:  cliTagPlan{current: "v0.10.0", previous: "v0.8.0", reuse: true},
		},
		{
			name:  "a tag on HEAD that is not a version does not count",
			state: cliTagState{headTags: []string{"v2-experiment"}, nearest: "v0.59.0"},
			level: cliBumpPatch,
			want:  cliTagPlan{current: "v0.59.1", previous: "v0.59.0"},
		},
		{
			name:  "the first release of a tagged first commit has no previous tag",
			state: cliTagState{headTags: []string{"v0.1.0"}, nearest: "v0.1.0"},
			level: cliBumpPatch,
			want:  cliTagPlan{current: "v0.1.0", reuse: true},
		},
		{
			name:    "no CLI tag is an error",
			state:   cliTagState{},
			level:   cliBumpPatch,
			wantErr: errNoCLITag,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planCLITag(tt.state, tt.level)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// newCLITestRepo makes a repository with one commit and a bare origin, and
// makes it the working directory. The git configuration of the machine is out
// of reach, so the result does not depend on it.
func newCLITestRepo(t *testing.T) {
	t.Helper()

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GITHUB_OUTPUT", "")

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.Mkdir(repo, 0o755))
	t.Chdir(repo)

	cliTestGit(t, "init", "--quiet", "--bare", origin)
	cliTestGit(t, "init", "--quiet", "--initial-branch=main")
	cliTestGit(t, "remote", "add", "origin", origin)
	cliTestCommit(t, "first")
}

func cliTestGit(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), output)
	return strings.TrimSpace(string(output))
}

func cliTestCommit(t *testing.T, message string) {
	t.Helper()
	cliTestGit(t, "commit", "--quiet", "--allow-empty", "-m", message)
}

func cliTestTag(t *testing.T, tag string) {
	t.Helper()
	cliTestGit(t, "tag", "-a", tag, "-m", tag)
}

func cliTestOriginTags(t *testing.T) []string {
	t.Helper()
	var tags []string
	for line := range strings.Lines(cliTestGit(t, "ls-remote", "--tags", "origin")) {
		ref := strings.Fields(line)[1]
		// The refs that end in ^{} are the commits that annotated tags point at.
		if !strings.HasSuffix(ref, "^{}") {
			tags = append(tags, strings.TrimPrefix(ref, "refs/tags/"))
		}
	}
	return tags
}

func TestReadCLITagState(t *testing.T) {
	t.Run("a tag of another program is not the nearest tag", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestCommit(t, "desktop release")
		cliTestTag(t, "desktop/v0.2.0")
		cliTestCommit(t, "work")

		state, err := readCLITagState(t.Context())
		require.NoError(t, err)
		assert.Empty(t, state.headTags)
		assert.Equal(t, "v0.1.0", state.nearest)
		assert.Equal(t, cliTestGit(t, "rev-parse", "HEAD"), state.head)
	})

	t.Run("a tag of another program on HEAD is not a head tag", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestCommit(t, "work")
		cliTestTag(t, "desktop/v9.9.9")

		state, err := readCLITagState(t.Context())
		require.NoError(t, err)
		assert.Empty(t, state.headTags)
		assert.Equal(t, "v0.1.0", state.nearest)
	})

	t.Run("a tag that is not an ancestor of HEAD is not the nearest tag", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestGit(t, "checkout", "--quiet", "-b", "side")
		cliTestCommit(t, "side work")
		cliTestTag(t, "v0.5.0")
		cliTestGit(t, "checkout", "--quiet", "main")
		cliTestCommit(t, "work")

		state, err := readCLITagState(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "v0.1.0", state.nearest)
	})

	t.Run("a tag on HEAD", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestCommit(t, "work")
		cliTestTag(t, "v0.1.1")

		state, err := readCLITagState(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"v0.1.1"}, state.headTags)
		assert.Equal(t, "v0.1.0", state.nearestParent)
	})

	t.Run("no CLI tag", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "desktop/v0.2.0")
		cliTestCommit(t, "work")

		state, err := readCLITagState(t.Context())
		require.NoError(t, err)
		assert.Empty(t, state.nearest)
		assert.Empty(t, state.nearestParent)
	})

	t.Run("the first commit has no parent", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")

		state, err := readCLITagState(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"v0.1.0"}, state.headTags)
		assert.Empty(t, state.nearestParent)
	})
}

func TestRunCLITag(t *testing.T) {
	run := func(t *testing.T, level cliBumpLevel, dryRun bool) (stdout, stderr string, err error) {
		t.Helper()
		var out, errOut bytes.Buffer
		err = runCLITag(context.WithoutCancel(t.Context()), level, dryRun, &out, &errOut)
		return out.String(), errOut.String(), err
	}

	t.Run("a dry run makes a local tag and pushes nothing", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestCommit(t, "work")

		stdout, _, err := run(t, cliBumpMinor, true)
		require.NoError(t, err)
		assert.Equal(t, "current=v0.2.0 previous=v0.1.0\n", stdout)
		assert.Equal(t, cliTestGit(t, "rev-parse", "HEAD"), cliTestGit(t, "rev-list", "-n", "1", "v0.2.0"))
		assert.Empty(t, cliTestOriginTags(t))
	})

	t.Run("a release pushes the tag", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.9.9")
		cliTestCommit(t, "work")

		stdout, _, err := run(t, cliBumpMajor, false)
		require.NoError(t, err)
		assert.Equal(t, "current=v1.0.0 previous=v0.9.9\n", stdout)
		assert.Equal(t, []string{"v1.0.0"}, cliTestOriginTags(t))
	})

	t.Run("a desktop tag on HEAD does not change the result", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.59.0")
		cliTestCommit(t, "work")
		cliTestTag(t, "desktop/v9.9.9")

		stdout, _, err := run(t, cliBumpMinor, true)
		require.NoError(t, err)
		assert.Equal(t, "current=v0.60.0 previous=v0.59.0\n", stdout)
	})

	t.Run("a tag on HEAD is used again", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestCommit(t, "work")
		cliTestTag(t, "v0.1.1")

		stdout, stderr, err := run(t, cliBumpMinor, true)
		require.NoError(t, err)
		assert.Equal(t, "current=v0.1.1 previous=v0.1.0\n", stdout)
		assert.Contains(t, stderr, "reusing it")
		assert.Equal(t, "v0.1.0\nv0.1.1", cliTestGit(t, "tag", "--list", "v*"))
	})

	t.Run("GITHUB_OUTPUT gets the two values", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestCommit(t, "work")
		output := filepath.Join(t.TempDir(), "github-output")
		t.Setenv("GITHUB_OUTPUT", output)

		_, _, err := run(t, cliBumpPatch, true)
		require.NoError(t, err)

		written, err := os.ReadFile(output)
		require.NoError(t, err)
		assert.Equal(t, "current=v0.1.1\nprevious=v0.1.0\n", string(written))
	})

	t.Run("no CLI tag is an error and makes no tag", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "desktop/v0.2.0")
		cliTestCommit(t, "work")

		_, _, err := run(t, cliBumpPatch, true)
		require.ErrorIs(t, err, errNoCLITag)
		assert.Empty(t, cliTestGit(t, "tag", "--list", "v*"))
	})

	t.Run("origin has the tag on a different commit", func(t *testing.T) {
		newCLITestRepo(t)
		cliTestTag(t, "v0.1.0")
		cliTestGit(t, "checkout", "--quiet", "-b", "elsewhere")
		cliTestCommit(t, "other work")
		cliTestTag(t, "v0.1.1")
		cliTestGit(t, "push", "--quiet", "origin", "refs/tags/v0.1.1")
		cliTestGit(t, "tag", "-d", "v0.1.1")
		cliTestGit(t, "checkout", "--quiet", "main")
		cliTestCommit(t, "work")

		_, _, err := run(t, cliBumpPatch, false)
		require.Error(t, err)
	})
}

func TestCLITagCommand(t *testing.T) {
	newCLITestRepo(t)
	cliTestTag(t, "v0.1.0")
	cliTestCommit(t, "work")

	ctx := context.WithoutCancel(t.Context())
	require.NoError(t, newReleaseCommand().Run(ctx, []string{"release", "cli", "tag", "--dry-run", "patch"}))
	assert.Equal(t, "v0.1.0\nv0.1.1", cliTestGit(t, "tag", "--list", "v*"))
	assert.Empty(t, cliTestOriginTags(t))

	require.Error(t, newReleaseCommand().Run(ctx, []string{"release", "cli", "tag", "--dry-run"}))
	require.Error(t, newReleaseCommand().Run(ctx, []string{"release", "cli", "tag", "--dry-run", "huge"}))
}
