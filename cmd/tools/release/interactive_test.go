package main

import (
	"strings"
	"testing"
)

func TestParseInteractiveReleaseArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		wantChannel   string
		wantCandidate string
		wantErr       string
	}{
		{name: "prompt for channel"},
		{name: "computed dev", args: []string{"dev"}, wantChannel: "dev"},
		{name: "explicit beta", args: []string{"beta", "1.2.3-beta.1"}, wantChannel: "beta", wantCandidate: "1.2.3-beta.1"},
		{name: "unknown channel", args: []string{"nightly"}, wantErr: "expected a channel"},
		{name: "too many", args: []string{"dev", "1.2.3-dev.1", "extra"}, wantErr: "optional channel"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			channel, candidate, err := parseInteractiveReleaseArgs(test.args)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("parseInteractiveReleaseArgs() error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if channel != test.wantChannel || candidate != test.wantCandidate {
				t.Fatalf("parseInteractiveReleaseArgs() = (%q, %q), want (%q, %q)", channel, candidate, test.wantChannel, test.wantCandidate)
			}
		})
	}
}

func TestRenderReleasePlan(t *testing.T) {
	t.Parallel()

	version, err := parsePublishVersion("0.2.0-dev.1")
	if err != nil {
		t.Fatal(err)
	}
	output := renderReleasePlan(releasePlan{
		version: version,
		channel: "dev",
		commit:  "abc123",
		subject: "Ship the release prompt",
		currentManifests: map[string]string{
			"dev": "0.1.8-dev.25",
		},
	})
	for _, want := range []string{
		"Release candidate",
		"0.2.0-dev.1",
		"abc123",
		"Ship the release prompt",
		"0.1.8-dev.25",
		"Current stable",
		"empty",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderReleasePlan() does not contain %q:\n%s", want, output)
		}
	}
}

func TestReleaseVersionForBump(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		candidate string
		bump      releaseBump
		want      string
	}{
		{name: "patch keeps normal dev progression", candidate: "0.1.8-dev.26", bump: bumpPatch, want: "0.1.8-dev.26"},
		{name: "minor resets dev prerelease", candidate: "0.1.8-dev.26", bump: bumpMinor, want: "0.2.0-dev.1"},
		{name: "major resets beta prerelease", candidate: "0.8.4-beta.3", bump: bumpMajor, want: "1.0.0-beta.1"},
		{name: "minor stable", candidate: "1.4.2", bump: bumpMinor, want: "1.5.0"},
		{name: "major stable", candidate: "1.4.2", bump: bumpMajor, want: "2.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate, err := parsePublishVersion(test.candidate)
			if err != nil {
				t.Fatal(err)
			}
			if got := releaseVersionForBump(candidate, test.bump).String(); got != test.want {
				t.Fatalf("releaseVersionForBump() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidateInteractiveReleaseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		channel string
		version string
		wantErr string
	}{
		{name: "dev", channel: "dev", version: " 1.2.3-dev.4 "},
		{name: "beta", channel: "beta", version: "1.2.3-beta.1"},
		{name: "stable", channel: "stable", version: "1.2.3"},
		{name: "wrong channel", channel: "stable", version: "1.2.3-dev.1", wantErr: "belongs to dev"},
		{name: "invalid", channel: "dev", version: "next", wantErr: "expected X.Y.Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateInteractiveReleaseVersion(test.channel, test.version)
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateInteractiveReleaseVersion() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}
