package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitStatusMock implements git.Git with configurable errors for testing fetchGitStatusForPath.
type gitStatusMock struct {
	branch    string
	branchErr error
	additions int
	deletions int
	diffErr   error
	isClean   bool
	cleanErr  error
}

func (m *gitStatusMock) Clone(context.Context, string, string) error           { return nil }
func (m *gitStatusMock) Checkout(context.Context, string, string) error        { return nil }
func (m *gitStatusMock) Pull(context.Context, string) error                    { return nil }
func (m *gitStatusMock) ResetHard(context.Context, string) error               { return nil }
func (m *gitStatusMock) RemoteURL(context.Context, string) (string, error)     { return "", nil }
func (m *gitStatusMock) DefaultBranch(context.Context, string) (string, error) { return "main", nil }
func (m *gitStatusMock) IsValidRepo(context.Context, string) error             { return nil }

func (m *gitStatusMock) Branch(context.Context, string) (string, error) {
	return m.branch, m.branchErr
}

func (m *gitStatusMock) DiffStats(context.Context, string) (int, int, error) {
	return m.additions, m.deletions, m.diffErr
}

func (m *gitStatusMock) IsClean(context.Context, string) (bool, error) {
	return m.isClean, m.cleanErr
}

func TestFetchGitStatusForPath(t *testing.T) {
	ctx := context.Background()
	errFail := errors.New("fail")

	t.Run("branch error", func(t *testing.T) {
		g := &gitStatusMock{branchErr: errFail}
		status := fetchGitStatusForPath(ctx, g, "/repo")
		assert.ErrorIs(t, status.Error, errFail)
	})

	t.Run("diff stats error", func(t *testing.T) {
		g := &gitStatusMock{branch: "main", diffErr: errFail}
		status := fetchGitStatusForPath(ctx, g, "/repo")
		assert.Equal(t, "main", status.Branch)
		assert.ErrorIs(t, status.Error, errFail)
	})

	t.Run("is clean error preserves prior fields", func(t *testing.T) {
		g := &gitStatusMock{branch: "main", additions: 1, deletions: 2, cleanErr: errFail}
		status := fetchGitStatusForPath(ctx, g, "/repo")
		require.ErrorIs(t, status.Error, errFail)
		assert.Equal(t, "main", status.Branch)
		assert.Equal(t, 1, status.Additions)
		assert.Equal(t, 2, status.Deletions)
	})

	t.Run("happy path clean repo", func(t *testing.T) {
		g := &gitStatusMock{branch: "feat/x", additions: 10, deletions: 3, isClean: true}
		status := fetchGitStatusForPath(ctx, g, "/repo")
		require.NoError(t, status.Error)
		assert.Equal(t, "feat/x", status.Branch)
		assert.Equal(t, 10, status.Additions)
		assert.Equal(t, 3, status.Deletions)
		assert.False(t, status.HasChanges)
	})

	t.Run("happy path dirty repo", func(t *testing.T) {
		g := &gitStatusMock{branch: "main", isClean: false}
		status := fetchGitStatusForPath(ctx, g, "/repo")
		require.NoError(t, status.Error)
		assert.True(t, status.HasChanges)
	})
}
