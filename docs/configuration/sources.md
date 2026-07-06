# Sources

!!! warning "Experimental"
    Sources are experimental — the API and config names may still change.

Sources let hive browse issues/PRs from GitHub **and** Gitea/Forgejo,
search/filter items, and create a session from a selected item using the same
batch-spawn path as `hive batch`. The forge backend is detected from the
repository's git remote host (see [Backends](#backends)).

## Configuration

```yaml
sources:
  search_limit: 30 # max items per search (default: 30)
  cache_ttl: 30s # search result cache TTL (default: 30s)
  hosts: # optional: override backend auto-detection per git remote host
    git.example.com: gitea
    github.acme.com: github
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
  built-in sources activate only if the backend's CLI — `gh` or `tea` — is on
  `PATH`), `true`/`false` explicitly enables/disables registration. The
  `enabled` flag applies to a source id across both backends.
- Every source configures `templates.name`, `templates.prompt`, and optional
  `templates.tags` — Go templates rendered against the selected item. Templates
  are shared across backends, so keep them forge-neutral (all backends populate
  `.Fields.number`, `.Fields.url`, and `.Detail`).
- Template data:
  - `.ID`, `.Title`, `.Subtitle`
  - `.Detail` — fetched markdown/detail content when available
  - `.Fields` — source-specific data (e.g. `.Fields.number`, `.Fields.url`
    for issues; `.Fields.review`, `.Fields.ci`, `.Fields.branch` for PRs)

## Backends

The forge backend for a repository is derived from its git remote host:

- `github.com` and hosts matching `github` → the **GitHub** backend (`gh`).
  Unrecognized hosts also default here, so GitHub Enterprise hosts reuse `gh`.
- `codeberg.org` and hosts matching `gitea`/`forgejo` → the **Gitea** backend
  (`tea`).
- `sources.hosts` overrides the detection for a specific host (`github` or
  `gitea`), for mirrored or ambiguously named self-hosted instances.

The picker only shows the backend that matches the selected session's remote,
and each backend requires its CLI to be installed and authenticated:

- **GitHub** — `gh auth login` (supports GitHub Enterprise via `gh`'s host
  config).
- **Gitea/Forgejo** — `tea login add` for the instance. hive runs `tea` in the
  session's local checkout so it resolves the matching login from the repo's
  remote; without a local checkout `tea` falls back to its default login.

## Built-in sources

| ID       | GitHub (`gh`)                     | Gitea (`tea`)      | Detail |
| -------- | --------------------------------- | ------------------ | ------ |
| `issues` | `gh issue list` / `gh issue view` | `tea issues list`  | yes¹   |
| `prs`    | `gh pr list`                      | `tea pulls list`   | no     |

¹ GitHub issues fetch the body via `gh issue view`; Gitea has no single-issue
JSON view, so `tea` returns the body inline and the picker uses it directly.

Built-ins are drivers (`internal/sources/ghcli`, `internal/sources/teacli`):
per-forge argv builders and JSON parsers executed by a shared engine
(`internal/sources/cliengine`). Gitea's `tea` list output is thinner than
GitHub's — it carries no CI rollup or review decision, so those card cells stay
blank for Gitea PRs.

## Opening a source

In the TUI, run `:SourceIssues [scope]` or `:SourcePRs [scope]` from the
command palette (scope defaults to the selected session's repo), or the
generic `:Sources <id> [scope]`. The default `i`/`p` keybindings in the
sessions view open issues/PRs respectively.

Picker keys: `j/k` or arrows move selection, `/` enters search mode (esc
returns to navigate, keeping the filter), `O` opens the highlighted item in
your browser, and enter creates a session from it using the source's configured
templates.

Space marks items for batch spawning: enter then creates one session per
marked item instead of the highlighted one. Marks survive filtering, tab
switches, and re-searches, and are capped at 10 across all tabs. Session
creation runs on a single background stream; if some items fail (e.g. a
template error), the rest still spawn and the completion notice reports how
many failed.

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
