package main

import "testing"

func TestNextVersion(t *testing.T) {
	t.Parallel()

	dev1 := mustParseVersion(t, "0.1.8-dev.1")
	beta1 := mustParseVersion(t, "0.1.8-beta.1")
	stable := mustParseVersion(t, "0.1.8")

	tests := []struct {
		name     string
		channel  string
		versions []releaseVersion
		want     string
	}{
		{name: "first dev", channel: "dev", want: "0.1.0-dev.1"},
		{name: "increment dev", channel: "dev", versions: []releaseVersion{dev1}, want: "0.1.8-dev.2"},
		{name: "promote dev to beta", channel: "beta", versions: []releaseVersion{dev1}, want: "0.1.8-beta.1"},
		{name: "promote beta to stable", channel: "stable", versions: []releaseVersion{beta1}, want: "0.1.8"},
		{name: "dev after beta advances patch", channel: "dev", versions: []releaseVersion{beta1}, want: "0.1.9-dev.1"},
		{name: "stable after stable advances patch", channel: "stable", versions: []releaseVersion{stable}, want: "0.1.9"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := nextVersion(test.channel, test.versions); got != test.want {
				t.Fatalf("nextVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"0.1.0", "10.20.30-beta.2", "desktop-v1.2.3-dev.4"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if _, err := parseVersion(value); err != nil {
				t.Fatalf("parseVersion(%q): %v", value, err)
			}
		})
	}
	for _, value := range []string{"", "v1.2.3", "1.2", "1.2.3-rc.1", "1.2.3-dev.0"} {
		t.Run("invalid-"+value, func(t *testing.T) {
			t.Parallel()
			if _, err := parseVersion(value); err == nil {
				t.Fatalf("parseVersion(%q) unexpectedly succeeded", value)
			}
		})
	}
}

func TestParsePublishVersionRejectsTagPrefix(t *testing.T) {
	t.Parallel()

	if _, err := parsePublishVersion("desktop-v1.2.3-dev.4"); err == nil {
		t.Fatal("parsePublishVersion unexpectedly accepted a tag")
	}
	if _, err := parsePublishVersion("1.2.3-dev.4"); err != nil {
		t.Fatalf("parsePublishVersion rejected a version: %v", err)
	}
}

func TestCompareChannelRelease(t *testing.T) {
	t.Parallel()

	ordered := []releaseVersion{
		mustParseVersion(t, "1.2.3-dev.4"),
		mustParseVersion(t, "1.2.3-beta.1"),
		mustParseVersion(t, "1.2.3"),
		mustParseVersion(t, "1.2.4-dev.1"),
	}
	for i := 1; i < len(ordered); i++ {
		if compareChannelRelease(ordered[i], ordered[i-1]) <= 0 {
			t.Fatalf("%s should advance %s", ordered[i], ordered[i-1])
		}
	}
}

func TestAffectedChannels(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"1.0.0":        {"stable", "beta", "dev"},
		"1.0.0-beta.1": {"beta", "dev"},
		"1.0.0-dev.1":  {"dev"},
	}
	for value, want := range tests {
		version := mustParseVersion(t, value)
		got := version.affectedChannels()
		if len(got) != len(want) {
			t.Fatalf("%s affected channels = %v, want %v", value, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s affected channels = %v, want %v", value, got, want)
			}
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
