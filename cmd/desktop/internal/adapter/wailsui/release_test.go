package wailsui

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShortCommit(t *testing.T) {
	require.Equal(t, "abc1234", ShortCommit("abc1234def567890"))
	require.Equal(t, "abc12", ShortCommit("abc12"))
	require.Equal(t, "HEAD", ShortCommit("HEAD"))
	require.Empty(t, ShortCommit(""))
}

func TestReleaseChannel(t *testing.T) {
	tests := []struct {
		version string
		channel string
		ok      bool
	}{
		{"1.2.3", "stable", true},
		{"v1.2.3", "stable", true},
		{"1.2.3-beta.1", "beta", true},
		{"1.2.3-dev.4", "dev", true},
		{"dev", "", false},
		{"", "", false},
		{"(devel)", "", false},
		{"v0.0.0-20240101000000-abcdef123456", "", false},
		{"1.2.3-rc.1", "", false},
		{"1.2.3-beta", "", false},
		{"1.2", "", false},
		// The prerelease number is an integer, matching what cmd/release can
		// actually publish — these are what the pattern used to admit.
		{"1.2.3-dev.1a", "", false},
		{"1.2.3-dev.1.2", "", false},
		{"1.2.3-dev.beta", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			channel, ok := ReleaseChannel(tt.version)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.channel, channel)
		})
	}
}

func TestSystemServiceBuild(t *testing.T) {
	// The package defaults: the short "HEAD" commit passes through untouched.
	info := NewSystemService(nil, "dev", "HEAD", "now").Build()
	require.Equal(t, "dev", info.Version)
	require.Equal(t, "HEAD", info.Commit)
	// An unreleased build has no channel, which is what About reads as "this
	// build cannot self-update".
	require.Empty(t, info.Channel)
	require.Equal(t, runtime.GOOS, info.OS)
	require.Equal(t, runtime.GOARCH, info.Arch)
	require.Equal(t, runtime.Version(), info.GoVersion)
}

func TestSystemServiceBuildChannel(t *testing.T) {
	require.Equal(t, "beta", NewSystemService(nil, "1.2.3-beta.1", "abc1234", "now").Build().Channel)
}
