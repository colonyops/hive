// Package teacli implements hive's built-in Gitea/Forgejo sources as
// cliengine drivers backed by the tea CLI.
//
// tea's list commands emit JSON as an array of flat, string-valued objects
// (every field is a string, including numbers and comma-joined lists), which
// is thinner than gh's GraphQL output. The drivers here parse that shape and
// map it into the same source Item fields the picker renders, leaving
// unavailable fields (CI status, review decision) blank.
//
// tea resolves which instance/login to talk to from the checkout's git
// remote, so the engine runs tea in the session's repo directory
// (SearchParams.Dir). Without a local checkout tea falls back to its default
// login, which may target the wrong host.
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

// firstCSV returns the first entry of a comma-joined value and the total
// count.
func firstCSV(s string) (first string, count int) {
	parts := splitCSV(s)
	if len(parts) == 0 {
		return "", 0
	}
	return parts[0], len(parts)
}

// teaAge parses tea's RFC3339 timestamp string and renders it as a compact
// age. An empty or unparseable value renders blank.
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
