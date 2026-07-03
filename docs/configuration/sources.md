# Sources

!!! warning "Experimental"
    Sources are an early vertical slice: built-in GitHub issues and pull
    requests, session templates, and a searchable picker. The API and config
    names may still change while source integrations mature.

Sources let hive browse GitHub issues/PRs, search/filter items, and create a
session from a selected item using the same batch-spawn path as `hive batch`.

## Configuration

```yaml
sources:
  issues:
    enabled: true # nil/omitted = auto, true/false = override
    templates:
      name: "gh-{{ .Fields.number }}-{{ .Title }}"
      prompt: |
        Work on {{ .Title }}

        {{ .Fields.url }}

        {{ .Detail }}
      tags: ["github", "issue-{{ .Fields.number }}"]
  prs:
    enabled: true
    templates:
      name: "gh-pr-{{ .Fields.number }}-{{ .Title }}"
      prompt: |
        Review pull request {{ .Title }}

        {{ .Fields.url }}
      tags: ["github", "pr-{{ .Fields.number }}"]
```

Rules:

- `enabled` follows the plugin convention: omitted means auto-detect (the
  built-in sources activate only if `gh` is on `PATH`), `true`/`false`
  explicitly enables/disables registration.
- Every source configures `templates.name`, `templates.prompt`, and optional
  `templates.tags` — Go templates rendered against the selected item.
- Template data:
  - `.ID`, `.Title`, `.Subtitle`
  - `.Detail` — fetched markdown/detail content when available
  - `.Fields` — source-specific data (e.g. `.Fields.number`, `.Fields.url`
    for issues; `.Fields.review`, `.Fields.ci`, `.Fields.branch` for PRs)

## Built-in sources

Built-ins use the GitHub CLI (`gh`) and the current auth/session configured
for that CLI.

| ID       | Data                            | Layout                  | Detail |
| -------- | ------------------------------- | ----------------------- | ------ |
| `issues` | `gh issue list` / `gh issue view` | two-line list + preview | yes    |
| `prs`    | `gh pr list`                    | table                   | no     |

Built-ins are declared as specs in `internal/sources/ghcli` — a small
declarative shape with gh argv builders and JSON parsers executed by the
shared engine.

## Opening a source

In the TUI, run `:SourceIssues [scope]` or `:SourcePRs [scope]` from the
command palette (scope defaults to the selected session's repo), or the
generic `:OpenSource <id> [scope]`. The default `i`/`p` keybindings in the
sessions view open issues/PRs respectively.

Picker keys: `j/k` or arrows move selection, `/` enters search mode (esc
returns to navigate, keeping the filter), `O` opens the highlighted item in
your browser, and enter creates a session from it using the source's configured
templates.

Custom commands can pin a source id and scope via preset `args`, and keybindings
can reference those commands. This is how the built-in `SourceIssues`/`SourcePRs`
commands are defined:

```yaml
usercommands:
  MyRepoIssues:
    action: OpenSourcePicker
    args: ["issues", "owner/repo"]
    scope: ["sessions"]
    silent: true

views:
  sessions:
    keybindings:
      I: { cmd: MyRepoIssues }
```

## Noninteractive CLI seam

```bash
hive source open issues --scope cli/cli --pick 12345 --json
```

`--pick` selects an item by ID from the source's search results (an empty
`--query` returns the source's default listing). On success this prints the
created session as JSON when `--json` is set; otherwise it prints a short
human-readable confirmation.
