package report

import (
	"net/url"
	"strings"
	"testing"
)

func queryOf(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse issue url: %v", err)
	}
	return u.Query()
}

func TestIssueURLPrefillsBuildIdentityOnly(t *testing.T) {
	q := queryOf(t, IssueURL(BuildInfo{
		Version:   "0.5.1",
		Channel:   "stable",
		Commit:    "abc1234",
		OS:        "darwin",
		Arch:      "arm64",
		GoVersion: "go1.25.1",
	}))

	if got := q.Get("template"); got != "bug-desktop.yml" {
		t.Errorf("template = %q", got)
	}
	if got := q.Get("version"); got != "0.5.1 (stable)" {
		t.Errorf("version = %q", got)
	}
	env := q.Get("environment")
	for _, want := range []string{"darwin/arm64", "go1.25.1", "commit abc1234"} {
		if !strings.Contains(env, want) {
			t.Errorf("environment missing %q: %q", want, env)
		}
	}
	// Anything else a bundle carries names the user or their work, and this
	// URL opens a public issue.
	for _, field := range []string{"description", "reproduction", "bundle"} {
		if q.Has(field) {
			t.Errorf("field %q should not be prefilled", field)
		}
	}
}

func TestIssueURLUnreleasedBuild(t *testing.T) {
	q := queryOf(t, IssueURL(BuildInfo{Version: "dev", OS: "linux", Arch: "amd64"}))
	if got := q.Get("version"); got != "built from source" {
		t.Errorf("version = %q", got)
	}
}
