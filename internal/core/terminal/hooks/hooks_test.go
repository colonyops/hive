package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Name(t *testing.T) {
	assert.Equal(t, "hooks", New().Name())
}

func TestIntegration_Available(t *testing.T) {
	assert.True(t, New().Available())
}

func TestIntegration_DiscoverSession(t *testing.T) {
	ctx := context.Background()
	h := New()

	t.Run("returns nil when no session path in metadata", func(t *testing.T) {
		info, err := h.DiscoverSession(ctx, "slug", map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("returns nil when status file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		info, err := h.DiscoverSession(ctx, "slug", map[string]string{
			sessionPathKey: dir,
		})
		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("returns SessionInfo when status file exists", func(t *testing.T) {
		dir := t.TempDir()
		statusFile := filepath.Join(dir, StatusFileName)
		require.NoError(t, os.WriteFile(statusFile, []byte("ready"), 0o644))

		info, err := h.DiscoverSession(ctx, "my-slug", map[string]string{
			sessionPathKey: dir,
		})
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "my-slug", info.Name)
		assert.Equal(t, dir, info.Pane)
	})
}

func TestIntegration_GetStatus(t *testing.T) {
	ctx := context.Background()
	h := New()

	t.Run("returns StatusMissing for nil info", func(t *testing.T) {
		status, err := h.GetStatus(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusMissing, status)
	})

	t.Run("returns StatusMissing when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusMissing, status)
	})

	t.Run("returns StatusReady for fresh ready status", func(t *testing.T) {
		dir := t.TempDir()
		writeStatus(t, dir, "ready")
		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusReady, status)
	})

	t.Run("returns StatusActive for fresh active status", func(t *testing.T) {
		dir := t.TempDir()
		writeStatus(t, dir, "active")
		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusActive, status)
	})

	t.Run("returns StatusApproval for approval status", func(t *testing.T) {
		dir := t.TempDir()
		writeStatus(t, dir, "approval")
		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusApproval, status)
	})

	t.Run("returns StatusMissing for unknown status value", func(t *testing.T) {
		dir := t.TempDir()
		writeStatus(t, dir, "unknown")
		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusMissing, status)
	})

	t.Run("returns StatusMissing for stale active status", func(t *testing.T) {
		dir := t.TempDir()
		statusFile := filepath.Join(dir, StatusFileName)
		require.NoError(t, os.WriteFile(statusFile, []byte("active"), 0o644))
		// Back-date the file beyond maxActiveAge
		oldTime := time.Now().Add(-(maxActiveAge + time.Second))
		require.NoError(t, os.Chtimes(statusFile, oldTime, oldTime))

		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusMissing, status)
	})

	t.Run("returns StatusMissing for stale ready status", func(t *testing.T) {
		dir := t.TempDir()
		statusFile := filepath.Join(dir, StatusFileName)
		require.NoError(t, os.WriteFile(statusFile, []byte("ready"), 0o644))
		oldTime := time.Now().Add(-(maxReadyAge + time.Second))
		require.NoError(t, os.Chtimes(statusFile, oldTime, oldTime))

		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusMissing, status)
	})

	t.Run("ignores trailing whitespace in status file", func(t *testing.T) {
		dir := t.TempDir()
		writeStatus(t, dir, "ready\n")
		info := &terminal.SessionInfo{Pane: dir}
		status, err := h.GetStatus(ctx, info)
		require.NoError(t, err)
		assert.Equal(t, terminal.StatusReady, status)
	})
}

// writeStatus writes a status string to the session's status file.
func writeStatus(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, StatusFileName), []byte(content), 0o644))
}
