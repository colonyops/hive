package report

import (
	"fmt"
	"net/url"
	"strings"
)

const IssueFormURL = "https://github.com/colonyops/hive/issues/new"

// IssueURL returns the bug form with the build identity filled in. Do not
// prefill another field: this URL opens a public issue, and every other
// surface a bundle carries names the user's machine, hosts or repositories.
func IssueURL(b BuildInfo) string {
	q := url.Values{}
	q.Set("template", "bug-desktop.yml")
	q.Set("version", versionField(b))
	q.Set("environment", environmentField(b))
	return IssueFormURL + "?" + q.Encode()
}

// versionField matches the wording bug-desktop.yml's Version field asks for.
func versionField(b BuildInfo) string {
	if b.Version == "" || b.Version == "dev" {
		return "built from source"
	}
	if b.Channel == "" {
		return b.Version
	}
	return fmt.Sprintf("%s (%s)", b.Version, b.Channel)
}

func environmentField(b BuildInfo) string {
	lines := []string{fmt.Sprintf("- %s/%s", b.OS, b.Arch)}
	if b.GoVersion != "" {
		lines = append(lines, "- "+b.GoVersion)
	}
	if b.Commit != "" {
		lines = append(lines, "- commit "+b.Commit)
	}
	return strings.Join(lines, "\n")
}
