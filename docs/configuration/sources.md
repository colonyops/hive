# Sources

!!! warning "Experimental"
    Sources are experimental — the API and config names may still change.

Sources let hive browse issues/PRs from GitHub **and** Gitea/Forgejo, as well
as records supplied by configured external subprocesses. You can search/filter
items and create a session from a selected item. The forge backend for built-in
sources is detected from the repository's git remote host (see
[Backends](#backends)).

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

## External subprocess sources

`sources.external` is an ordered YAML list of one-shot JSON-RPC source
adapters. Declaration order is preserved, and external tabs follow built-ins
and saved views.

```yaml
sources:
  external:
    - name: firing-alerts
      command: [hive-source-gcx-alerts]
      env:
        # Only ${NAME} references are expanded from hive's environment.
        GCX_CONTEXT: "${GCX_CONTEXT}"
      timeout: 30s
      templates:
        name: "alert-{{ .ID }}-{{ .Fields.target }}"
        prompt: |
          Investigate {{ .Title }}.
          State: {{ .Fields.state }}
          Service: {{ .Fields.service }}
          URL: {{ .Fields.url }}
          Labels: {{ .Fields.labels }}
        tags: [alert, "{{ .Fields.signal_slug }}"]
```

This example expects `hive-source-gcx-alerts` to translate a gcx-style firing
alert query into the protocol below. The adapter, rather than hive, owns the
gcx command and response schema. It should invoke gcx with a direct argument
vector, not by interpolating alert data into a shell command.

Each external entry has:

- `name` — unique source id. It may contain letters, numbers, dashes, and
  underscores, and must not collide with `issues`, `prs`, or a saved view.
- `command` — nonempty argv list. hive executes `command[0]` directly with the
  remaining entries as arguments; it does not invoke a shell.
- `env` — optional environment overrides. The child inherits hive's complete
  environment. In override values, `${NAME}` references expand from hive's
  parent environment and missing variables become empty. `$NAME`, command
  substitution, and other shell syntax are not evaluated.
- `timeout` — optional duration for each request. The default is 15 seconds;
  the maximum configured value is 24 hours. The example uses 30 seconds for a
  remote CLI query.
- `templates` — the same `name`, `prompt`, and `tags` Go templates used by
  built-ins. External item `.Fields` are adapter-defined.

External sources are registered for both GitHub and Gitea/Forgejo backends, so
they remain visible regardless of the selected repository's forge. Availability
only checks whether `command[0]` is on `PATH`; authentication and service errors
are reported when the adapter handles a request.

### One-shot JSON-RPC protocol

hive starts a fresh subprocess for every method call. It writes exactly one
newline-terminated JSON-RPC 2.0 request to stdin and closes stdin. The adapter
must write exactly one newline-terminated JSON-RPC 2.0 response to stdout.
Extra stdout lines, a missing final newline, a mismatched integer id, or a
response containing both/neither `result` and `error` are protocol errors. Put
human-readable diagnostics and logs on stderr, never stdout.

The methods are:

- `initialize` with `{}` params. Its result is a manifest:

  ```json
  {"id":"firing-alerts","displayName":"Firing alerts","capabilities":{"fetchDetail":false},"picker":{"search":{"debounceMS":500}}}
  ```

  hive enforces the configured source `name` as the final manifest id. A zero
  or omitted `debounceMS` uses the picker's default.

- `search` with `query`, `scope`, `dir`, and `cursor` string params. Its result
  contains `items` and an optional `nextCursor`:

  ```json
  {"items":[{"id":"alert-123","title":"Error budget burn","subtitle":"Alerting · api","uri":"https://alerts.example/items/123","fields":{"state":"Alerting","service":"api","target":"api"}}],"nextCursor":""}
  ```

  Item `id` values must be stable. `title`, `subtitle`, and `uri` are strings;
  `uri` may be empty; and `fields` is an arbitrary JSON object exposed to
  templates as `.Fields`. The picker performs an initial search and sends
  interactive query changes after the manifest's debounce interval. Adapters
  choose whether to pass the query to their backend or filter locally. They may
  ignore `scope`, `dir`, and `cursor` when those concepts do not apply.

- `fetchDetail` with `id`, `scope`, `uri`, and `dir` string params. Implement it
  only when `capabilities.fetchDetail` is true. Its result is
  `{"markdown":{"content":"..."}}`; use a JSON-RPC method-not-found error
  when unsupported.

Adapters should return standard JSON-RPC error objects for invalid methods or
params and a server error for dependency failures, timeouts, or malformed
backend data. hive also bounds subprocess stdout/stderr and surfaces nonzero
exit status and stderr without treating it as protocol output.

### Picker and repository behavior

Unlike repository-scoped built-ins, an external item never chooses the clone
remote, even if its fields or URI resemble a repository. Selecting one opens
the New Session form and requires manual confirmation from repositories already
discovered in configured workspaces. If there are no discovered repositories,
hive returns to the picker with an error.

External results can be marked, but session creation is deliberately
one-at-a-time because each result needs repository confirmation. If multiple
items are marked, hive preserves the marks and asks you to unmark all but one.
Interactive search remains enabled for external tabs.

### Security

Treat adapters as trusted local executables: they receive hive's inherited
environment, working context in request params, and any configured overrides.
Use an absolute or otherwise controlled executable on `PATH`, keep argv fixed,
and avoid shells. Do not put credentials directly in YAML; reference a narrowly
scoped environment variable instead. Adapters should validate remote data,
bound errors, reserve stdout for the single protocol response, and allow-list
URLs before returning them.

External titles, fields, detail, and rendered prompts are also remote-content
and prompt-injection surfaces. Apply the same trust guidance as built-in
sources, and remember that labels or annotations included in a prompt are fed
to the selected agent verbatim.

## Views (saved searches)

`sources.views` defines named, fixed GitHub searches over the `issues` or `prs`
built-in source. It is a YAML **list**, not a map, so declaration order is
preserved:

```yaml
sources:
  search_limit: 30
  cache_ttl: 30s
  issues:
    templates:
      name: "issue-{{ .Fields.repo }}-{{ .Fields.number }}"
      prompt: "Work on {{ .Fields.url }}: {{ .Title }}"
      tags: ["issue"]
  prs:
    templates:
      name: "review-{{ .Fields.repo }}-{{ .Fields.number }}"
      prompt: "Review {{ .Fields.url }}: {{ .Title }}"
      tags: ["review"]
  views:
    # Global: searches every repository visible to gh.
    - name: my-review-queue
      base: prs
      query: "review-requested:@me state:open"

    # Scoped: limits the search to one repository.
    - name: triage
      base: issues
      query: "label:triage no:assignee archived:false"
      scope: "colonyops/hive"
```

Each list item has these fields:

- `name` is the unique source id, tab label, and input to generated command
  naming.
- `base` is `issues` or `prs`. The view inherits that built-in's
  `templates.name`, `templates.prompt`, and `templates.tags`.
- `query` is passed verbatim to `gh search issues` or `gh search prs`. hive
  does not parse, normalize, or add qualifiers.
- `scope` is an optional `owner/repo`. When present, hive adds the repository
  restriction; when omitted, the search is global across repositories visible
  to the authenticated `gh` user.

Views also inherit the source-wide `search_limit` and `cache_ttl`. There are no
per-view template, limit, or cache overrides yet. Tabs appear after the built-in
Issues and Pull Requests tabs, in list declaration order.

A saved-view tab always executes its configured query, so interactive picker
search is disabled on that tab. Selecting an item from a scoped view uses the
normal instant-spawn flow. Selecting an item from a global view opens the New
Session form for confirmation: hive constructs or reuses the selected item's
repository remote, preselects it, and generates an editable session name from
the inherited base template. Submitting the form still renders the inherited
prompt and tags for the selected item. The [remote-content security
warning](#configuration) applies to saved views too, especially global views
that can return items from repositories you did not anticipate.

### Commands and keybindings

Every view gets a generated command named `Source<PascalCaseName>`. For example,
`my-review-queue` generates `SourceMyReviewQueue`, which can be run as
`:SourceMyReviewQueue`. The generic `:Sources my-review-queue` form opens the
same view.

A keybinding can reference the generated command directly; its preset command
arguments already contain the view name:

```yaml
views:
  sessions:
    keybindings:
      R: { cmd: SourceMyReviewQueue, help: "my review queue" }
```

For a custom command name, put the view id in the command's `args`, then bind
that command:

```yaml
usercommands:
  ReviewQueue:
    action: OpenSourcePicker
    args: ["my-review-queue"]
    scope: ["sessions"]
    silent: true

views:
  sessions:
    keybindings:
      R: { cmd: ReviewQueue }
```

Command collisions are resolved predictably and produce configuration warnings:
a user command overrides a generated command; a built-in command overrides a
generated command; and if multiple view names normalize to the same generated
name, the first view in declaration order wins.

### GitHub behavior and errors

Saved views are GitHub-only in v1. They use `gh search` and do not appear as
tabs for Gitea/Forgejo repositories. A config containing views still loads when
`gh` is unavailable; hive emits a non-fatal availability warning instead.

hive adds no special handling for archived repositories or forks. Put the
policy in the query itself: for example, add `archived:false` to exclude
archived results and use GitHub-supported qualifiers for any fork policy.
`gh search` uses GitHub's search API, which is limited to 30 requests per minute
for authenticated users. A rate-limit or other `gh search` failure appears as
the picker's error state; hive does not retry it. Successful results are served
from the cache within `sources.cache_ttl`, avoiding another API request until
that entry expires.

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
