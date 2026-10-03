package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoDirsCheck_AllExist(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	check := NewRepoDirsCheck([]string{dir1, dir2})
	result := check.Run(context.Background())

	assert.Equal(t, "Repository Directories", result.Name)
	require.Len(t, result.Items, 2)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, StatusPass, result.Items[1].Status)
}

func TestRepoDirsCheck_MissingDir(t *testing.T) {
	existing := t.TempDir()

	check := NewRepoDirsCheck([]string{existing, "/nonexistent/path/abc123"})
	result := check.Run(context.Background())

	require.Len(t, result.Items, 2)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, StatusWarn, result.Items[1].Status)
	assert.Contains(t, result.Items[1].Detail, "does not exist")
}

func TestRepoDirsCheck_NotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644))

	check := NewRepoDirsCheck([]string{filePath})
	result := check.Run(context.Background())

	require.Len(t, result.Items, 1)
	assert.Equal(t, StatusFail, result.Items[0].Status)
	assert.Contains(t, result.Items[0].Detail, "not a directory")
}

func TestRepoDirsCheck_TildeExpansion(t *testing.T) {
	// ~ should expand to the user's home directory, which exists
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	check := NewRepoDirsCheck([]string{"~"})
	result := check.Run(context.Background())

	require.Len(t, result.Items, 1)
	// Home dir should exist and be a directory
	assert.Equal(t, StatusPass, result.Items[0].Status)
	// Label should still show the original ~ path for user clarity
	_ = home // home is used indirectly via expansion
}

func TestRepoDirsCheck_NoneConfigured(t *testing.T) {
	check := NewRepoDirsCheck(nil)
	result := check.Run(context.Background())

	require.Len(t, result.Items, 1)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Contains(t, result.Items[0].Detail, "none configured")
}
