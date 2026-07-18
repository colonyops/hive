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
- sources: list of GitHub data acquisitions. Sources are the only part of
  the config that costs API requests; identical sources are deduplicated.
  - id: unique lowercase slug across all sources.
  - kind: "search" or "notifications".
  - query: required for kind "search"; GitHub issue/PR search syntax
    (e.g. "is:open is:pr author:@me archived:false"). Forbidden for
    kind "notifications", which mirrors the GitHub notifications inbox.
  - limit: optional items per fetch. Search: default 50, max 100.
    Notifications: default 50, max 50 (API cap).
- profiles: list of workspaces shown in the app rail.
  - id: unique lowercase slug. Stable — renaming it makes it a new profile.
  - name: display name.
  - feeds: list of feeds in the workspace. A feed is a client-side
    filtered view over sources — feeds are unlimited and free of API cost.
    - id: unique slug within the profile.
    - name: display name.
    - sources: list of source ids to read from (at least one; every id
      must exist under top-level sources). A feed over several sources
      merges and deduplicates their items.
    - filters: optional block. Groups AND together; values within a group
      OR; exclude groups win over includes. All groups optional:
      - repos / exclude_repos: doublestar globs matched against
        "owner/repo" (e.g. "acme/*", "acme/**").
      - authors / exclude_authors: globs matched against the item author,
        case-insensitively; "[" and "]" match literally, so
        exclude_authors: ["*[bot]"] drops every bot account.
      - labels / exclude_labels: globs; an item matches when ANY of its
        labels matches ANY glob (exclude: dropped when any label matches).
      - types: "pr" | "issue".
      - reasons: notification reasons (mention, review_requested, assign,
        author, comment, subscribed, team_mention, state_change, and the
        other GitHub reasons). Items that came only from a search source
        have no reason, so a reasons filter excludes them — reasons only
        make sense on feeds reading a notifications source.

Constraints:
- At most 25 sources of kind "search": each distinct search source is one
  request per poll (about once a minute) against GitHub's search rate
  limit of 30 requests/min, and 25 leaves headroom for manual refreshes.
- Notifications sources are not capped: they poll the core rate bucket
  (5000/hr) with conditional requests, and an unchanged inbox answers 304
  at no rate-limit cost.
- Feeds are unlimited and free: filtering happens client-side after the
  fetch. Prefer a few broad sources shared by many filtered feeds over
  many narrow search sources.

Current config:
`)
	b.WriteString("```yaml\n")
	b.WriteString(strings.TrimRight(info.YAML, "\n"))
	b.WriteString("\n```\n\n")
	b.WriteString("What I want: <describe your workspaces and feeds here>\n")
	return b.String()
}
