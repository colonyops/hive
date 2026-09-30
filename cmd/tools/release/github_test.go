package main

import (
	"slices"
	"strings"
	"testing"
)

func mustVersion(t *testing.T, value string) releaseVersion {
	t.Helper()
	version, err := parsePublishVersion(value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return version
}

func TestReleaseNotesHeader(t *testing.T) {
	header := releaseNotesHeader(mustVersion(t, "1.4.0-dev.2"), "https://dl.hivedesktop.com")
	for _, want := range []string{
		"dev channel",
		"https://dl.hivedesktop.com/desktop/releases/1.4.0-dev.2/",
		"https://dl.hivedesktop.com/desktop/releases/1.4.0-dev.2/SHA256SUMS",
	} {
		if !strings.Contains(header, want) {
			t.Fatalf("header missing %q:\n%s", want, header)
		}
	}
}

func TestReleaseTitle(t *testing.T) {
	if got := releaseTitle(mustVersion(t, "1.4.0-beta.1")); got != "Hive Desktop 1.4.0-beta.1" {
		t.Fatalf("title = %q", got)
	}
}

func TestGitHubReleaseCreateArgs(t *testing.T) {
	tests := []struct {
		version    string
		prerelease bool
	}{
		{version: "1.4.0", prerelease: false},
		{version: "1.4.0-beta.1", prerelease: true},
		{version: "1.4.0-dev.2", prerelease: true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			tag := "desktop-v" + tt.version
			args := gitHubReleaseCreateArgs(mustVersion(t, tt.version), tag, "notes body")

			if want := []string{"release", "create", tag}; !slices.Equal(args[:3], want) {
				t.Fatalf("args start with %q, want %q", args[:3], want)
			}
			for _, flag := range []string{"--verify-tag", "--latest=false"} {
				if !slices.Contains(args, flag) {
					t.Fatalf("args missing %s: %q", flag, args)
				}
			}
			if got := slices.Contains(args, "--prerelease"); got != tt.prerelease {
				t.Fatalf("--prerelease present = %t, want %t: %q", got, tt.prerelease, args)
			}
			if i := slices.Index(args, "--notes"); i < 0 || args[i+1] != "notes body" {
				t.Fatalf("--notes not followed by the body: %q", args)
			}
		})
	}
}
