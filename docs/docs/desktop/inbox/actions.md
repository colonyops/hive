---
icon: lucide/play
description: Configure reusable actions and quick terminal launchers.
---

# Actions

Actions are reusable operations defined in `actions.yml`. Configure them under **Settings ▸ Actions** or edit the file directly.

Actions can appear on feed items, terminal sessions, terminal windows, or in a flow. A flow that only needs to start a session or chat can use a [launch node](flows.md#start-work-from-a-flow) instead.

## Action types

- **`launch-session`** starts a repository coding session or an agent workspace chat from an item.
- **`shell`** runs a shell command.
- **`publish-message`** sends a message to a topic.
- **`clipboard`** renders text and copies it.

A manual item action can limit itself to kinds such as pull requests or issues. An action can also ask for text or a selected value before running.

## Example

```yaml
version: 1
actions:
  - id: review-pr
    label: Review PR
    type: launch-session
    applies_to: [pr]
    show_in_detail: true
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Review pull request {{ .Payload.title }}

      {{ .Payload.url }}
launchers: []
```

This action appears for pull requests and starts a session with the repository and prompt rendered from the item.

Use `workspace` instead of `repo_template` to send an item to an agent workspace:

```yaml
- id: triage-alert
  label: Triage alert
  type: launch-session
  workspace: incident-triage
  prompt_template: |
    Triage {{ .Payload.alert }} in {{ .Payload.cluster }}.

    {{ .Payload.thread_url }}
```

`workspace` is the workspace directory name. The workspace command must pass
`.Prompt` through `shq`, as the shipped command presets do. Hive refuses the
launch if the command would drop the item context or interpolate it as shell
syntax. `repo_template` and `workspace` cannot appear together.

Template fields depend on where the action runs:

- Item actions use `{{ .Payload.<field> }}` and `{{ .Key }}`. `{{ .ItemRemote }}` is the clone URL of the item's repository, for `repo_template`.
- Session actions use values such as `{{ .Session.Path }}` and `{{ .Session.Branch }}`.
- Window actions can also use `{{ .Window.ID }}`.
- Declared inputs use `{{ .Inputs.<name> }}`.

Use the `shq` template function when inserting item or input data into a shell command.

### Fence outside content in a prompt

A pull request body, an alert, or a webhook payload can contain text that reads like instructions. Wrap it in `{{ untrustedStart }}` and `{{ untrustedEnd }}` so the agent can tell where your prompt stops and the item starts:

```yaml
prompt_template: |
  Triage this alert.

  {{ untrustedNotice }}

  {{ untrustedStart "source" "grafana" }}
  {{ .Payload.title }}
  {{ .Payload.url }}

  {{ .Payload.body }}
  {{ untrustedEnd }}
```

The helpers render an opening and closing tag, `<untrusted-content-<id>>` and `</untrusted-content-<id>>`. The id changes on every run, so a closing tag copied into the item's text cannot close the fence. `{{ untrustedNotice }}` renders a sentence that tells the agent what the tags mean, so put it before the fence. Pass key/value pairs to `untrustedStart` to add attributes to the opening tag, such as `{{ untrustedStart "source" "grafana" "kind" .Payload.kind }}`. Attribute values are escaped. The same helpers work in a notify node's templates.

Hive does not fence anything for you. Every value goes into the prompt exactly as the template writes it, so decide for each field:

| Field | Who wrote it | Fence it in a prompt? |
| --- | --- | --- |
| `{{ .Payload.<field> }}`, `{{ .Raw }}` | The source: a PR author, an issue reporter, an alerting tool, a webhook sender | Yes, including short fields like `title` and `author` |
| `{{ .Payload.repo }}`, `{{ .Payload.num }}`, `{{ .Payload.url }}` on a GitHub or Gitea item | The forge, which fixes their shape | Optional. They can stay outside the fence to tell the agent what to work on |
| `{{ .Key }}` | Built from the item's id | Yes for webhook items, whose sender picks the id |
| `{{ .Inputs.<name> }}` | You, when you run the action | No |
| `{{ .Session.* }}`, `{{ .Window.ID }}` | Hive | No |

Fence only text an agent reads, such as `prompt_template`. In a shell command a fence does nothing; quote values with `shq` instead. The starter actions and the New Session draft built from an item already fence the item's text.

## Run a command after the session starts

A `launch-session` action can run a `post_hook` once the session exists. The command runs in the new checkout with your shell's `PATH`, so it can check the pull request out and open your editor on it:

```yaml
- id: review-pr
  label: Review PR
  type: launch-session
  applies_to: [pr]
  repo_template: "https://github.com/{{ .Payload.repo }}.git"
  prompt_template: "Review pull request #{{ .Payload.num }}"
  post_hook: "gh pr checkout {{ .Payload.num }} && zed ."
  post_hook_timeout: "2m"
```

The hook reads the same template fields as the action, plus `{{ .Session.Path }}` and `{{ .Session.Slug }}` for the session that was just created. `post_hook_timeout` defaults to one minute. A hook that fails leaves the session in place and reports its output in the action's run log.

## Where actions run

Use `targets` to choose where an action appears:

```yaml
- id: run-tests
  label: Run tests
  type: shell
  targets: [session]
  timeout: "10m"
  command_template: "mise run test"
```

`item` is the default target. `session` and `window` add the action to row menus in Code. A `launch-session` action only supports item targets.

Item actions run independently. A long shell action stays **Running** until its command exits, while later manual and flow actions can continue. Hiding the window leaves it running; quitting Hive cancels it. Shell actions capture output but do not provide terminal input.

## Read what a run did

**Action runs** lists every run of an `actions.yml` action, newest first, whether you started it from an item or a flow started it. Open it from the views menu in the title bar, with <kbd>g</kbd> then <kbd>l</kbd>, from the command palette, from **View log** on an item's action, or from the jobs menu.

Select a run to read its log: the command line, its stdout and stderr in the order they were written, and the exit status. A session launch or a published message logs the steps it took instead. A running action's log updates as it runs, and you can cancel it from there. Automatic retries appear as separate attempts.

Each attempt keeps up to 512 KiB of output, and the newest 200 runs keep their logs. Notify and launch nodes, and actions run from a terminal session or window, do not appear here.

A flow runs its named action for every routed item, regardless of `applies_to`. Flow actions cannot use the clipboard. A `launch-session` flow action needs `repo_template` or `workspace`, and required inputs need defaults.

A manual `launch-session` action with neither fixed target opens a dialog where
you can choose a repository or an agent workspace. `agent` and `post_hook` apply
only to repository sessions. A fixed workspace target cannot use either field.

## Quick terminals

Quick terminals open a tool in the pop-up terminal. Configure them under **Settings ▸ Quick terminals** or in the `launchers` list:

```yaml
launchers:
  - id: lazygit
    label: lazygit
    command: lazygit
```

A launcher without `cwd` uses the active terminal directory. A launcher with `cwd` is available from anywhere. Launchers also appear in the command palette and can have keyboard shortcuts.

!!! tip "Ask the Hive workspace"
    The **Hive** workspace in **Chats** can update `actions.yml` with the `hive-actions` skill.
