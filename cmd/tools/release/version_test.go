package main

import "testing"

func TestNextVersion(t *testing.T) {
	t.Parallel()

	history := []string{"0.58.0", "0.59.0", "0.9.0", "0.9.1-dev.3"}
	tests := []struct {
		name      string
		published []string
		level     bumpLevel
		want      string
	}{
		{name: "minor continues the CLI line past the desktop", published: history, level: bumpMinor, want: "0.60.0"},
		{name: "patch", published: history, level: bumpPatch, want: "0.59.1"},
		{name: "major", published: history, level: bumpMajor, want: "1.0.0"},
		{name: "a shared release is the newest", published: append(history, "0.60.0"), level: bumpMinor, want: "0.61.0"},
		{name: "a desktop prerelease newer than every tag", published: []string{"0.59.0", "0.59.1-dev.2"}, level: bumpPatch, want: "0.59.2"},
		{name: "nothing published", level: bumpMinor, want: "0.1.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			published := make([]releaseVersion, 0, len(test.published))
			for _, value := range test.published {
				published = append(published, mustParseVersion(t, value))
			}
			if got := nextVersion(published, test.level); got.String() != test.want {
				t.Fatalf("nextVersion() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestParseBumpLevel(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"patch", "minor", "major"} {
		if _, err := parseBumpLevel(value); err != nil {
			t.Fatalf("parseBumpLevel(%q): %v", value, err)
		}
	}
	if _, err := parseBumpLevel("stable"); err == nil {
		t.Fatal("expected an unknown bump level to be rejected")
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"0.1.0", "10.20.30-beta.2", "desktop-v1.2.3-dev.4", "v0.59.0", "v0.60.0"} {
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

	if _, err := parsePublishVersion("0.60.0"); err != nil {
		t.Fatalf("parsePublishVersion rejected a version: %v", err)
	}
	for _, value := range []string{"v0.60.0", "desktop-v1.2.3", "1.2.3-dev.4", "1.2.3-beta.1"} {
		if _, err := parsePublishVersion(value); err == nil {
			t.Fatalf("parsePublishVersion(%q) unexpectedly succeeded", value)
		}
	}
}

func TestReleaseVersionTag(t *testing.T) {
	t.Parallel()

	if got := mustParseVersion(t, "0.60.2").tag(); got != "v0.60.2" {
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
		mustParseVersion(t, "0.60.0"),
		mustParseVersion(t, "0.60.1"),
		mustParseVersion(t, "0.61.0"),
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
