//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// branchTemplateConfig returns inline YAML config with a catch-all worktree rule
// and the given branch_template. Uses spawn instead of windows so the test
// doesn't need a running tmux server.
func branchTemplateConfig(tmpl string) string {
	return fmt.Sprintf(`data_dir: ""
rules:
  - clone_strategy: worktree
    branch_template: %q
    spawn:
      - "tmux new-session -d -s {{ .Slug | shq }} -c {{ .Path | shq }}"
`, tmpl)
}

// bareRepoBranches returns the branch names in the bare repo that backs the
// given worktree session path. worktreeBareDir already resolves to the bare root.
func bareRepoBranches(t *testing.T, sessionPath string) []string {
	t.Helper()
	bareRoot := worktreeBareDir(t, sessionPath)
	out, err := exec.Command("git", "--git-dir", bareRoot, "branch", "--format=%(refname:short)").CombinedOutput()
	require.NoError(t, err, "git branch: %s", out)
	var branches []string
	for _, b := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if b != "" {
			branches = append(branches, b)
		}
	}
	return branches
}

func TestBranchTemplate_RendersSlugAndID(t *testing.T) {
	h := NewHarness(t).WithConfig(branchTemplateConfig("jalevin/{{ .Slug }}-{{ .ID }}"))
	repo := createBareRepo(t, "bt-slug-repo")

	out, err := h.Run("new", "--remote", repo, "my-feature")
	require.NoError(t, err, "hive new: %s", out)

	path := parseCreatedSessionPath(t, out)
	branches := bareRepoBranches(t, path)

	var found bool
	for _, b := range branches {
		if regexp.MustCompile(`^jalevin/my-feature-[a-z0-9]+$`).MatchString(b) {
			found = true
			break
		}
	}
	assert.True(t, found, "expected jalevin/my-feature-<id> branch, got: %v", branches)
}

func TestBranchTemplate_DefaultIsHiveSlugID(t *testing.T) {
	h := NewHarness(t).WithConfig(`data_dir: ""
rules:
  - clone_strategy: worktree
    spawn:
      - "tmux new-session -d -s {{ .Slug | shq }} -c {{ .Path | shq }}"
`)
	repo := createBareRepo(t, "bt-default-repo")

	out, err := h.Run("new", "--remote", repo, "my-feature")
	require.NoError(t, err, "hive new: %s", out)

	path := parseCreatedSessionPath(t, out)
	branches := bareRepoBranches(t, path)

	var found bool
	for _, b := range branches {
		if regexp.MustCompile(`^hive/my-feature-[a-z0-9]+$`).MatchString(b) {
			found = true
			break
		}
	}
	assert.True(t, found, "expected hive/my-feature-<id> branch, got: %v", branches)
}

func TestBranchTemplate_InvalidRenderedNameReturnsError(t *testing.T) {
	// .Name with a space produces "feat/My Feature" — invalid git branch name
	h := NewHarness(t).WithConfig(branchTemplateConfig("feat/{{ .Name }}"))
	repo := createBareRepo(t, "bt-invalid-repo")

	out, err := h.Run("new", "--remote", repo, "My Feature")
	require.Error(t, err, "hive new with invalid branch name should fail")
	assert.Contains(t, out, "invalid git branch name")
	assert.Contains(t, out, "feat/My Feature")
	assert.Contains(t, out, "spaces")
}

func TestBranchTemplate_NameWithSlashPreservesInBranch(t *testing.T) {
	// Using .Name directly when the session name is already branch-safe
	h := NewHarness(t).WithConfig(branchTemplateConfig("{{ .Name }}"))
	repo := createBareRepo(t, "bt-slash-repo")

	out, err := h.Run("new", "--remote", repo, "jalevin/my-feature")
	require.NoError(t, err, "hive new with slash name: %s", out)

	path := parseCreatedSessionPath(t, out)
	branches := bareRepoBranches(t, path)
	assert.Contains(t, branches, "jalevin/my-feature", "branch should preserve / from session name")
}

func TestBranchTemplate_CommandsReceiveTemplateVars(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "hook-output.txt")
	cfg := fmt.Sprintf(`data_dir: ""
rules:
  - clone_strategy: worktree
    branch_template: "jalevin/{{ .Slug }}-{{ .ID }}"
    commands:
      - "echo NAME={{ .Name | shq }} SLUG={{ .Slug }} OWNER={{ .Owner }} REPO={{ .Repo }} ID={{ .ID }} >> %s"
    spawn:
      - "tmux new-session -d -s {{ .Slug | shq }} -c {{ .Path | shq }}"
`, outFile)

	h := NewHarness(t).WithConfig(cfg)
	repo := createBareRepo(t, "bt-commands-repo")

	out, err := h.Run("new", "--remote", repo, "my feature")
	require.NoError(t, err, "hive new: %s", out)

	contentBytes, err := os.ReadFile(outFile)
	require.NoError(t, err, "hook output file must exist")
	content := string(contentBytes)
	assert.Contains(t, content, "NAME=my feature")
	assert.Contains(t, content, "SLUG=my-feature")
	assert.Contains(t, content, "REPO=bt-commands-repo")
	assert.Regexp(t, `ID=[a-z0-9]+`, content)
}

func TestSessionName_InvalidCharsRejected(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "bt-badname-repo")

	cases := []struct {
		name    string
		wantErr string
	}{
		{"feat~bad", "invalid characters"},
		{"feat^1", "invalid characters"},
		{"feat*", "invalid characters"},
		{"feat?", "invalid characters"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := h.Run("new", "--remote", repo, tc.name)
			require.Error(t, err, "hive new %q should fail", tc.name)
			assert.Contains(t, out, tc.wantErr)
		})
	}
}

func TestSessionName_ValidCharsAccepted(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "bt-goodname-repo")

	cases := []string{
		"my-feature",
		"my feature",
		"JIRA-123",
		"v1.2.3",
		"jalevin/test-thing",
		"my_feature",
	}

	for i, name := range cases {
		t.Run(name, func(t *testing.T) {
			// Each needs a unique name to avoid duplicate detection
			uniqueName := fmt.Sprintf("%s-%d", name, i)
			out, err := h.Run("new", "--remote", repo, uniqueName)
			require.NoError(t, err, "hive new %q should succeed: %s", uniqueName, out)
			assert.Contains(t, out, "Session created")
		})
	}
}
