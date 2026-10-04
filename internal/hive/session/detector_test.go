package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionDetector_DetectSessionFromPath(t *testing.T) {
	store := newMockStore()
	for _, s := range []session.Session{
		{ID: "sess-1", Path: "/home/user/projects/foo", State: session.StateActive},
		{ID: "sess-2", Path: "/home/user/projects/bar", State: session.StateActive},
		{ID: "sess-3", Path: "/home/user/projects/foo/nested", State: session.StateActive},
		{ID: "recycled", Path: "/home/user/projects/old", State: session.StateRecycled},
	} {
		store.sessions[s.ID] = s
	}

	detector := NewDetector(store)
	ctx := context.Background()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "exact match",
			path:     "/home/user/projects/foo",
			expected: "sess-1",
		},
		{
			name:     "subdirectory match",
			path:     "/home/user/projects/foo/src/main.go",
			expected: "sess-1",
		},
		{
			name:     "nested session takes precedence",
			path:     "/home/user/projects/foo/nested/deep/file.go",
			expected: "sess-3",
		},
		{
			name:     "different session",
			path:     "/home/user/projects/bar/code",
			expected: "sess-2",
		},
		{
			name:     "no match",
			path:     "/home/user/other/project",
			expected: "",
		},
		{
			name:     "recycled session ignored",
			path:     "/home/user/projects/old/file.txt",
			expected: "",
		},
		{
			name:     "partial name match is not a match",
			path:     "/home/user/projects/foobar",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detector.DetectSessionFromPath(ctx, tt.path)
			require.NoError(t, err, "DetectSessionFromPath failed: %v", err)
			assert.Equal(t, tt.expected, got, "DetectSessionFromPath(%q) = %q, want %q", tt.path, got, tt.expected)
		})
	}
}

func TestSessionDetector_EmptyStore(t *testing.T) {
	store := newMockStore()
	detector := NewDetector(store)
	ctx := context.Background()

	got, err := detector.DetectSessionFromPath(ctx, "/any/path")
	require.NoError(t, err, "DetectSessionFromPath failed: %v", err)
	assert.Empty(t, got, "DetectSessionFromPath with empty store = %q, want empty", got)
}

func TestIsSubpath(t *testing.T) {
	tests := []struct {
		parent   string
		child    string
		expected bool
	}{
		{"/home/user", "/home/user/projects", true},
		{"/home/user", "/home/user", false}, // Equal, not sub
		{"/home/user", "/home/username", false},
		{"/home/user/projects", "/home/user", false},
		{"/a/b/c", "/a/b/c/d/e/f", true},
	}

	for _, tt := range tests {
		t.Run(tt.parent+"->"+tt.child, func(t *testing.T) {
			got := isSubpath(tt.parent, tt.child)
			assert.Equal(t, tt.expected, got, "isSubpath(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.expected)
		})
	}
}

func TestSessionDetector_SessionAtPath(t *testing.T) {
	store := newMockStore()
	store.sessions["sess-1"] = session.Session{ID: "sess-1", Name: "fix-login", Path: "/home/user/projects/foo", State: session.StateActive}
	store.sessions["recycled"] = session.Session{ID: "recycled", Path: "/home/user/projects/old", State: session.StateRecycled}
	detector := NewDetector(store)

	sess, ok, err := detector.SessionAtPath(context.Background(), "/home/user/projects/foo/src")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "fix-login", sess.Name)

	for _, path := range []string{"/home/user/projects/old", "/home/user/projects"} {
		_, ok, err = detector.SessionAtPath(context.Background(), path)
		require.NoError(t, err)
		assert.False(t, ok, path)
	}
}

func TestSessionDetector_SessionAtPathThroughSymlink(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "repos", "foo")
	require.NoError(t, os.MkdirAll(filepath.Join(checkout, "src"), 0o755))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(checkout, link))

	store := newMockStore()
	store.sessions["sess-1"] = session.Session{ID: "sess-1", Path: checkout, State: session.StateActive}
	detector := NewDetector(store)

	sess, ok, err := detector.SessionAtPath(context.Background(), filepath.Join(link, "src"))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "sess-1", sess.ID)

	store.sessions["sess-1"] = session.Session{ID: "sess-1", Path: link, State: session.StateActive}
	sess, ok, err = detector.SessionAtPath(context.Background(), checkout)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "sess-1", sess.ID)
}
