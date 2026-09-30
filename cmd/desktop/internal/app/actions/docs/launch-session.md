# Launch session

A **launch-session** action starts a repository-backed Hive coding session or
an agent workspace chat from the triggering item. It is the action type behind
manual item handoffs and flow `action` nodes that start agents automatically.

## Fields

- `prompt_template` (required) — the new session's initial prompt.
- `repo_template` — which repository the Hive session is created against.
- `workspace` — the directory name of an agent workspace under the configured
  workspace root.
- `agent` — a non-default Hive agent profile (e.g. `claude`, `aider`). Omit for
  the launcher's default. Agent workspaces select their agent in `command:` and
  cannot use this field.
- `post_hook` — a shell command to run once a repository session exists, in its
  checkout. A workspace target cannot use it. See below.
- `post_hook_timeout` — how long the hook may run, as a duration string
  (`"2m"`). Defaults to one minute.

`repo_template` and `workspace` are mutually exclusive. Either fixed target
makes the action **headless**, so a flow `action` node can fire it with no human
present. With neither field, manual invocation asks for a repository or agent
workspace plus the session name. A flow rejects that interactive variant.

A workspace action resolves the current workspace definition when it runs. Its
`command:` must carry `.Prompt` through `shq`; otherwise Hive refuses the launch
rather than dropping the item context or treating it as shell syntax. The
shipped command presets already satisfy this requirement.

```yaml
- id: triage-alert
  label: Triage alert
  type: launch-session
  workspace: incident-triage
  prompt_template: |
    Triage {{ .Payload.alert }} in {{ .Payload.cluster }}.

    {{ .Payload.thread_url }}
```

## Post hook

`post_hook` runs after the session is created, through `sh -c`, with the new
session's checkout as the working directory and the login shell's environment
(so `gh`, `zed`, and the rest of your `PATH` resolve). It is the place to put
the setup the agent's prompt cannot do — check out the pull request the item is
about, then open an editor on it:

```yaml
- id: review-pr
  label: Review PR
  type: launch-session
  applies_to: [pr]
  repo_template: "https://github.com/{{ .Payload.repo }}.git"
  prompt_template: "Review pull request #{{ .Payload.num }}"
  post_hook: "gh pr checkout {{ .Payload.num }} && zed ."
```

The hook is rendered over the same data as the other templates, plus
`.Session`, bound to the session that was just created: `{{ .Session.Path }}`,
`{{ .Session.Slug }}` (its tmux session name), `{{ .Session.Name }}`,
`{{ .Session.ID }}`, and `{{ .Session.Repo }}`. `{{ .Session.Branch }}` is
empty here — a fresh session has no branch to report, and the hook is a shell
in the checkout already.

A hook that fails does **not** fail the action: the session exists by then, so
reporting the launch as failed would be untrue and would invite a retry that
creates a second session. Its exit status and output land in the action's run
log instead, under the item it ran for.

## Item target only

A launch-session action creates a *new* session, so it cannot declare
`targets: [session]` or `targets: [window]` — the terminal's row menus have no
New Session form to collect the interactive variant's repository and name, and
the headless variant's `repo_template` renders over a feed item's payload,
which a terminal target carries none of. Declaring one is rejected when the
catalog is parsed.

## Templates

`prompt_template` and `repo_template` are Go `text/template` strings rendered
over the triggering message (see "Template data" above). A GitHub-shaped item
exposes `{{ .Payload.repo }}`, `{{ .Payload.title }}`, `{{ .Payload.url }}`,
`{{ .Payload.body }}`, `{{ .Payload.num }}`, and `{{ .Payload.author }}`. A
webhook-sourced item exposes whatever its payload carries, so reshape it with a
`function` node first if you want stable names.
