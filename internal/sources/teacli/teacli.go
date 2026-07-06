// Package teacli implements hive's built-in Gitea/Forgejo sources as
// cliengine drivers backed by the tea CLI.
//
// tea's list JSON is stringly-typed: every field is a string, including
// numbers and comma-joined lists. Fields gh provides but tea doesn't (CI
// status, review decision) stay blank.
//
// tea resolves which instance/login to talk to from the checkout's git
// remote, so the engine runs tea in the repo directory (SearchParams.Dir).
// Without a local checkout tea falls back to its default login, which may
// target the wrong host.
package teacli

import (
	"strings"
	"time"

	"github.com/colonyops/hive/internal/sources/cliengine"
)

// splitCSV splits tea's comma-joined field values (labels, assignees) into
// trimmed, non-empty entries.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func firstCSV(s string) (first string, count int) {
	parts := splitCSV(s)
	if len(parts) == 0 {
		return "", 0
	}
	return parts[0], len(parts)
}

// teaAge renders tea's RFC3339 timestamp string as a compact age; an
// unparseable value renders blank.
func teaAge(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return cliengine.ShortAge(t)
}
