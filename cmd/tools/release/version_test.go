package main

import (
	"strings"
	"testing"
	"time"
)

func TestNextVersion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 1, 23, 30, 0, 0, time.UTC)
	tests := []struct {
		name      string
		published []string
		want      string
	}{
		{name: "nothing published", want: "0.20261001.0"},
		{name: "above the CLI and desktop history", published: []string{"0.59.0", "0.9.0", "0.9.1-dev.3"}, want: "0.20261001.0"},
		{name: "second release that day", published: []string{"0.59.0", "0.20261001.0"}, want: "0.20261001.1"},
		{name: "counts past a gap", published: []string{"0.20261001.0", "0.20261001.4"}, want: "0.20261001.5"},
		{name: "a new day starts at 0", published: []string{"0.20260930.3"}, want: "0.20261001.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			published := make([]releaseVersion, 0, len(test.published))
			for _, value := range test.published {
				published = append(published, mustParseVersion(t, value))
			}
			got, err := nextVersion(now, published)
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != test.want {
				t.Fatalf("nextVersion() = %s, want %s", got, test.want)
			}
		})
	}
}

// The date is UTC so two maintainers in different timezones cannot cut the
// same release under two dates.
func TestNextVersionUsesTheUTCDate(t *testing.T) {
	t.Parallel()

	chicago := time.FixedZone("CDT", -5*60*60)
	got, err := nextVersion(time.Date(2026, 10, 1, 20, 0, 0, 0, chicago), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "0.20261002.0" {
		t.Fatalf("nextVersion() = %s, want the UTC date 0.20261002.0", got)
	}
}

func TestNextVersionRefusesToGoBackwards(t *testing.T) {
	t.Parallel()

	_, err := nextVersion(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), []releaseVersion{mustParseVersion(t, "0.20261005.0")})
	if err == nil || !strings.Contains(err.Error(), "does not advance") {
		t.Fatalf("expected a clock behind the newest release to be refused, got %v", err)
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"0.1.0", "10.20.30-beta.2", "desktop-v1.2.3-dev.4", "v0.59.0", "v0.20261001.0"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if _, err := parseVersion(value); err != nil {
				t.Fatalf("parseVersion(%q): %v", value, err)
			}
		})
	}
	for _, value := range []string{"", "1.2", "1.2.3-rc.1", "1.2.3-dev.0", "v2-experiment"} {
		t.Run("invalid-"+value, func(t *testing.T) {
			t.Parallel()
			if _, err := parseVersion(value); err == nil {
				t.Fatalf("parseVersion(%q) unexpectedly succeeded", value)
			}
		})
	}
}

func TestParsePublishVersion(t *testing.T) {
	t.Parallel()

	if _, err := parsePublishVersion("0.20261001.0"); err != nil {
		t.Fatalf("parsePublishVersion rejected a version: %v", err)
	}
	for _, value := range []string{"v0.20261001.0", "desktop-v1.2.3", "1.2.3-dev.4", "1.2.3-beta.1"} {
		if _, err := parsePublishVersion(value); err == nil {
			t.Fatalf("parsePublishVersion(%q) unexpectedly succeeded", value)
		}
	}
}

func TestReleaseVersionTag(t *testing.T) {
	t.Parallel()

	if got := mustParseVersion(t, "0.20261001.2").tag(); got != "v0.20261001.2" {
		t.Fatalf("tag() = %q", got)
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	ordered := []releaseVersion{
		mustParseVersion(t, "0.9.0-dev.4"),
		mustParseVersion(t, "0.9.0-beta.1"),
		mustParseVersion(t, "0.9.0"),
		mustParseVersion(t, "0.59.0"),
		mustParseVersion(t, "0.20261001.0"),
		mustParseVersion(t, "0.20261001.1"),
		mustParseVersion(t, "0.20261002.0"),
	}
	for i := 1; i < len(ordered); i++ {
		if compareVersions(ordered[i], ordered[i-1]) <= 0 {
			t.Fatalf("%s should advance %s", ordered[i], ordered[i-1])
		}
	}
}

func mustParseVersion(t *testing.T, value string) releaseVersion {
	t.Helper()
	version, err := parseVersion(value)
	if err != nil {
		t.Fatal(err)
	}
	return version
}
