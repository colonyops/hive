package feed

import (
	"fmt"
	"strings"
)

// BuildConfigPrompt renders a paste-ready prompt for a coding agent to
// create or edit the profiles config on the user's behalf — the
// "config-as-code via agent" path for users who would rather describe their
// feeds than hand-write YAML. It carries the schema contract and the
// current config so the agent needs no other context.
func BuildConfigPrompt(info ConfigInfo) string {
	var b strings.Builder
	b.WriteString("Edit my Hive Desktop feed configuration.\n\n")
	fmt.Fprintf(&b, "File: %s\n", info.Path)
	if !info.Exists {
		b.WriteString("The file does not exist yet — create it.\n")
	}
	b.WriteString("The app watches this file and hot-reloads on save; no restart needed.\n\n")
	b.WriteString(`Schema (strict YAML — unknown keys are rejected):
- profiles: list of workspaces shown in the app rail.
  - id: unique lowercase slug. Stable — renaming it makes it a new profile.
  - name: display name.
  - feeds: list of feeds in the workspace.
    - id: unique slug within the profile.
    - name: display name.
    - kind: "search" or "notifications".
    - query: required for kind "search"; GitHub issue/PR search syntax
      (e.g. "is:open is:pr author:@me archived:false"). Forbidden for
      kind "notifications", which mirrors the GitHub notifications inbox.
    - repos: optional list of "owner/repo" globs to keep (e.g. "acme/*").
    - exclude_repos: optional list of globs to drop; excludes win.

Constraints:
- Each feed costs one GitHub API request per poll (about once a minute).
  Keep the feed list lean; configs with more than 30 feeds are rejected
  because they would exceed GitHub's search rate limit (30 requests/min).
- repos/exclude_repos filter client-side after fetching — free of API
  cost, so prefer one broad query plus glob filters over many narrow
  query feeds.

Current config:
`)
	b.WriteString("```yaml\n")
	b.WriteString(strings.TrimRight(info.YAML, "\n"))
	b.WriteString("\n```\n\n")
	b.WriteString("What I want: <describe your workspaces and feeds here>\n")
	return b.String()
}
