# Sources

!!! warning "Experimental"
    Sources are experimental — the API and config names may still change.

Sources let hive browse GitHub issues/PRs, search/filter items, and create a
session from a selected item using the same batch-spawn path as `hive batch`.

## Configuration

```yaml
sources:
  search_limit: 30 # max items per search (default: 30)
  cache_ttl: 30s # search result cache TTL (default: 30s)
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

!!! warning "Remote content in templates"
    Source items put **remote-authored** text (issue/PR titles and bodies)
    into `.Title`, `.Detail`, and the rendered `.Prompt`. Any spawn or
    window command template that interpolates `.Prompt` runs through a
    shell, so it must quote it with `shq` (the defaults do) — see the
    [rules documentation](rules.md) for details. Also note the rendered
    prompt is handed to your agent verbatim: opening a session from a
    public-repo issue feeds that issue's body to the agent, which is a
    prompt-injection surface. Prefer scoping sources to repositories you
    trust.

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
| `issues` | `gh issue list` / `gh issue view` | two-line card | yes    |
| `prs`    | `gh pr list`                    | two-line card | no     |

Built-ins are drivers in `internal/sources/ghcli`: gh argv builders and
JSON parsers executed by a shared engine.

## Opening a source

In the TUI, run `:SourceIssues [scope]` or `:SourcePRs [scope]` from the
command palette (scope defaults to the selected session's repo), or the
generic `:Sources <id> [scope]`. The default `i`/`p` keybindings in the
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
