package main

import (
	"strings"
	"testing"
	"time"
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
	header := releaseNotesHeader(mustVersion(t, "0.20261001.0"), "https://dl.hivedesktop.com")
	for _, want := range []string{
		"https://dl.hivedesktop.com/desktop/releases/0.20261001.0/",
		"https://dl.hivedesktop.com/desktop/releases/0.20261001.0/SHA256SUMS",
	} {
		if !strings.Contains(header, want) {
			t.Fatalf("header missing %q:\n%s", want, header)
		}
	}
}

// gh lists recent runs, some from before this dispatch. The run to watch is
// the newest one created after it.
func TestNewestRunAfter(t *testing.T) {
	dispatched := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	output := `[
		{"databaseId": 1, "createdAt": "2026-10-01T11:00:00Z"},
		{"databaseId": 3, "createdAt": "2026-10-01T12:00:09Z"},
		{"databaseId": 2, "createdAt": "2026-10-01T12:00:04Z"}
	]`

	id, ok := newestRunAfter(output, dispatched)
	if !ok || id != "3" {
		t.Fatalf("newestRunAfter() = %q, %t; want 3", id, ok)
	}
	if _, ok := newestRunAfter(`[{"databaseId": 1, "createdAt": "2026-10-01T11:00:00Z"}]`, dispatched); ok {
		t.Fatal("a run from before the dispatch must not match")
	}
}
