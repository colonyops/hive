package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShortCommit(t *testing.T) {
	require.Equal(t, "abc1234", shortCommit("abc1234def567890"))
	require.Equal(t, "abc12", shortCommit("abc12"))
	require.Equal(t, "HEAD", shortCommit("HEAD"))
	require.Empty(t, shortCommit(""))
}

func TestReleaseURL(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"released semver", "1.2.3", "https://github.com/colonyops/hive/releases/tag/desktop-v1.2.3"},
		{"leading v tolerated", "v0.4.0", "https://github.com/colonyops/hive/releases/tag/desktop-v0.4.0"},
		{"surrounding whitespace", "  1.0.0 ", "https://github.com/colonyops/hive/releases/tag/desktop-v1.0.0"},
		{"dev build", "dev", ""},
		{"empty", "", ""},
		{"pseudo version", "v0.0.0-20240101000000-abcdef123456", ""},
		{"non-numeric", "1.2.x", ""},
		{"missing patch", "1.2", ""},
		{"devel", "(devel)", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, releaseURL(tt.version))
		})
	}
}

func TestSystemServiceBuild(t *testing.T) {
	// The package defaults ("dev") mean no release link is offered, and the
	// short "HEAD" commit passes through untouched.
	info := NewSystemService().Build()
	require.Equal(t, "dev", info.Version)
	require.Equal(t, "HEAD", info.Commit)
	require.Equal(t, "https://github.com/colonyops/hive", info.RepoURL)
	require.Empty(t, info.ReleaseURL)
}
