package main

import (
	"strings"
	"testing"
)

func TestRenderReleasePlan(t *testing.T) {
	t.Parallel()

	version, err := parsePublishVersion("0.60.0")
	if err != nil {
		t.Fatal(err)
	}
	output := renderReleasePlan(releasePlan{
		version: version,
		commit:  "abc123",
		subject: "Ship the release prompt",
		currentManifests: map[string]string{
			"dev": "0.9.1-dev.25",
		},
	})
	for _, want := range []string{
		"Release candidate",
		"0.60.0",
		"v0.60.0",
		"abc123",
		"Ship the release prompt",
		"0.9.1-dev.25",
		"Current stable",
		"empty",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderReleasePlan() does not contain %q:\n%s", want, output)
		}
	}
}
