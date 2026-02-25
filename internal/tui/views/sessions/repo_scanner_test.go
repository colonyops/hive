package sessions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGit struct {
	remotes map[string]string // path -> remote URL
}

func (m *mockGit) Clone(context.Context, string, string) error                  { return nil }
func (m *mockGit) Checkout(context.Context, string, string) error               { return nil }
func (m *mockGit) Pull(context.Context, string) error                           { return nil }
func (m *mockGit) ResetHard(context.Context, string) error                      { return nil }
func (m *mockGit) IsClean(context.Context, string) (bool, error)                { return true, nil }
func (m *mockGit) Branch(context.Context, string) (string, error)               { return "main", nil }
func (m *mockGit) DefaultBranch(context.Context, string) (string, error)        { return "main", nil }
func (m *mockGit) DiffStats(context.Context, string) (int, int, error)          { return 0, 0, nil }
func (m *mockGit) IsValidRepo(context.Context, string) error                    { return nil }
func (m *mockGit) CloneBare(context.Context, string, string) error              { return nil }
func (m *mockGit) WorktreeAdd(context.Context, string, string, string) error    { return nil }
func (m *mockGit) WorktreeRemove(context.Context, string, string, string) error { return nil }
func (m *mockGit) Fetch(context.Context, string) error                          { return nil }
func (m *mockGit) RemoteURL(_ context.Context, dir string) (string, error) {
	if remote, ok := m.remotes[dir]; ok {
		return remote, nil
	}
	return "", os.ErrNotExist
}

func TestScanRepoDirs(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create test repos
	repos := []struct {
		name   string
		remote string
	}{
		{"alpha-repo", "git@github.com:user/alpha.git"},
		{"beta-repo", "git@github.com:user/beta.git"},
		{"zebra-repo", "https://github.com/user/zebra.git"},
	}

	gitMock := &mockGit{remotes: make(map[string]string)}

	for _, r := range repos {
		repoPath := filepath.Join(tmpDir, r.name)
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755))
		gitMock.remotes[repoPath] = r.remote
	}

	// Create a non-repo directory (no .git)
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "not-a-repo"), 0o755))

	// Create a file (should be skipped)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "somefile.txt"), []byte("test"), 0o644))

	ctx := context.Background()
	discovered, err := ScanRepoDirs(ctx, []string{tmpDir}, gitMock)
	require.NoError(t, err)

	assert.Len(t, discovered, 3)

	// Should be sorted alphabetically
	assert.Equal(t, "alpha-repo", discovered[0].Name)
	assert.Equal(t, "git@github.com:user/alpha.git", discovered[0].Remote)

	assert.Equal(t, "beta-repo", discovered[1].Name)
	assert.Equal(t, "git@github.com:user/beta.git", discovered[1].Remote)

	assert.Equal(t, "zebra-repo", discovered[2].Name)
	assert.Equal(t, "https://github.com/user/zebra.git", discovered[2].Remote)
}

func TestScanRepoDirs_EmptyDirs(t *testing.T) {
	ctx := context.Background()
	discovered, err := ScanRepoDirs(ctx, nil, &mockGit{})
	require.NoError(t, err)
	assert.Empty(t, discovered)
}

func TestScanRepoDirs_NonexistentDir(t *testing.T) {
	ctx := context.Background()
	discovered, err := ScanRepoDirs(ctx, []string{"/nonexistent/path"}, &mockGit{})
	require.NoError(t, err)
	assert.Empty(t, discovered)
}

func TestScanRepoDirs_MultipleDirs(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	gitMock := &mockGit{remotes: make(map[string]string)}

	// Create repo in first dir
	repo1 := filepath.Join(tmpDir1, "repo1")
	require.NoError(t, os.MkdirAll(filepath.Join(repo1, ".git"), 0o755))
	gitMock.remotes[repo1] = "git@github.com:user/repo1.git"

	// Create repo in second dir
	repo2 := filepath.Join(tmpDir2, "repo2")
	require.NoError(t, os.MkdirAll(filepath.Join(repo2, ".git"), 0o755))
	gitMock.remotes[repo2] = "git@github.com:user/repo2.git"

	ctx := context.Background()
	discovered, err := ScanRepoDirs(ctx, []string{tmpDir1, tmpDir2}, gitMock)
	require.NoError(t, err)

	assert.Len(t, discovered, 2)
	// Sorted alphabetically
	assert.Equal(t, "repo1", discovered[0].Name)
	assert.Equal(t, "repo2", discovered[1].Name)
}
