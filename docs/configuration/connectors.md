---
icon: lucide/plug
---

# Connectors

!!! warning "Vertical slice"
    Connectors are an early vertical slice: one built-in connector (GitHub
    issues), a custom JSON-RPC subprocess protocol for external connectors,
    and a two-pane picker. Scope is intentionally minimal — see the plan
    referenced from the epic for what's out of scope.

Connectors let hive browse an external system (GitHub issues, or a custom
subprocess speaking the connector protocol), search/filter items, and create
a session from a selected item using the same batch-spawn path as `hive
batch`.

## Configuration

```yaml
connectors:
  github:
    enabled: true
    templates:
      name: "gh-{{ .Fields.number }}-{{ .Title }}"
      prompt: "Work on {{ .Title }}\n\n{{ .Fields.url }}"
      tags:
        - "github"
        - "issue-{{ .Fields.number }}"
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
  (the GitHub connector activates only if `gh` is on `PATH`), `true`/`false`
  force it on or off.
- Every connector configures `templates.name`, `templates.prompt`, and
  optional `templates.tags` — Go templates rendered against the selected
  item just before session creation. `.ID`, `.Title`, `.Subtitle`, and
  `.Detail` (plain text) are always available; `.Fields.<key>` exposes
  connector-specific data (e.g. `.Fields.number`, `.Fields.url` for GitHub
  issues). A template referencing a `.Fields` key the item doesn't have
  fails at render time (not at config load time), since field names are
  connector-specific.
- `external` connectors declare a `command` — the subprocess hive spawns
  once per RPC call (`initialize`/`search`/`fetchDetail`), speaking a small
  JSON-RPC 2.0 protocol over newline-delimited stdio. `internal/connectors/rpc`
  implements the client side; `cmd/hive-reference-connector` is a canned
  example server useful for testing and as a protocol reference.

## GitHub issues

The built-in GitHub connector shells out to `gh issue list`/`gh issue view`
and requires a scope in `owner/name` form (there is no default — you supply
it when opening the connector). It needs `gh` installed and authenticated.

## Opening a connector

In the TUI, run `:OpenConnector <id> [scope]` from the command palette (e.g.
`:OpenConnector github cli/cli`). This opens a two-pane picker: matching
items on the left, detail (markdown or key/value) on the right. Press enter
to create a session from the highlighted item, using the connector's
configured templates.

There's a noninteractive CLI equivalent, primarily intended for scripting
and testing:

```bash
hive connector open github --scope cli/cli --pick 12345 --json
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
