---
icon: lucide/plug
---

# Connectors

!!! warning "Vertical slice"
    Connectors are an early vertical slice: two built-in connectors (GitHub
    issues and pull requests), a custom JSON-RPC subprocess protocol for
    external connectors, and a searchable picker. Scope is intentionally
    minimal — see the plan referenced from the epic for what's out of scope.

Connectors let hive browse an external system (GitHub issues/PRs, or a
custom subprocess speaking the connector protocol), search/filter items, and
create a session from a selected item using the same batch-spawn path as
`hive batch`.

## Configuration

```yaml
connectors:
  issues:
    enabled: true
    templates:
      name: "gh-{{ .Fields.number }}-{{ .Title }}"
      prompt: "Work on {{ .Title }}\n\n{{ .Fields.url }}"
      tags:
        - "github"
        - "issue-{{ .Fields.number }}"
  prs:
    enabled: true
    templates:
      name: "gh-pr-{{ .Fields.number }}-{{ .Title }}"
      prompt: "Review pull request {{ .Title }}\n\n{{ .Fields.url }}"
      tags:
        - "github"
        - "pr-{{ .Fields.number }}"
  external:
    - id: reference
      command: ["hive-reference-connector"]
      templates:
        name: "ref-{{ .ID }}"
        prompt: "{{ .Detail }}"
        tags:
          - "status-{{ .Fields.status }}"
```

- `enabled` follows the same convention as plugins: `nil`/omitted auto-detects
  (the built-in connectors activate only if `gh` is on `PATH`), `true`/`false`
  force it on or off.
- Every connector configures `templates.name`, `templates.prompt`, and
  optional `templates.tags` — Go templates rendered against the selected
  item just before session creation. `.ID`, `.Title`, `.Subtitle`, and
  `.Detail` (plain text) are always available; `.Fields.<key>` exposes
  connector-specific data (e.g. `.Fields.number`, `.Fields.url` for issues;
  PRs additionally expose `.Fields.branch`, `.Fields.draft`, and
  `.Fields.review`). A template referencing a `.Fields` key the item doesn't have
  fails at render time (not at config load time), since field names are
  connector-specific.
- `external` connectors declare a `command` — the subprocess hive spawns
  once per RPC call (`initialize`/`search`/`fetchDetail`), speaking a small
  JSON-RPC 2.0 protocol over newline-delimited stdio. `internal/connectors/rpc`
  implements the client side; `cmd/hive-reference-connector` is a canned
  example server useful for testing and as a protocol reference.

## Built-in connectors

Both built-ins shell out to the `gh` CLI and require a scope in
`owner/name` form (auto-detected from the selected session's git remote, or
supplied explicitly). They need `gh` installed and authenticated.

- **`issues`** — `gh issue list`/`gh issue view`. Two-pane picker: issue
  list on the left, markdown detail on the right.
- **`prs`** — `gh pr list`. Single-pane, full-width table (number, title,
  author, review state) with no detail preview.

Built-ins are declared as specs in `internal/connectors/ghcli` — a small
struct describing the picker layout and the gh invocation — executed by a
shared engine, so new gh-backed connectors are mostly declarative.

## Opening a connector

In the TUI, run `:ConnectorIssues [scope]` or `:ConnectorPRs [scope]` from
the command palette (scope defaults to the selected session's repo), or the
generic `:OpenConnector <id> [scope]`. The default `i` keybinding in the
sessions view opens the issues picker. Press enter to create a session from
the highlighted item, using the connector's configured templates.

There's a noninteractive CLI equivalent, primarily intended for scripting
and testing:

```bash
hive connector open issues --scope cli/cli --pick 12345 --json
```

`--pick` selects an item by ID from the connector's search results (an empty
`--query` returns the connector's default listing). On success this prints
the created session as JSON and exits 0; an unknown `--pick` ID exits
non-zero with an error and creates no session.

## Manual testing with the reference connector

```bash
go build -o /usr/local/bin/hive-reference-connector ./cmd/hive-reference-connector
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | hive-reference-connector
```

Configure it as an `external` connector (see above) to exercise the full
picker → template render → session-create path without needing a real
external system.
