---
icon: lucide/play
description: Configure reusable actions and quick terminal launchers.
---

# Actions

Actions are reusable operations defined in `actions.yml`. Configure them under **Settings ▸ Actions** or edit the file directly.

Actions can appear on feed items, terminal sessions, terminal windows, or in a flow.

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

- Item actions use `{{ .Payload.<field> }}` and `{{ .Key }}`.
- Session actions use values such as `{{ .Session.Path }}` and `{{ .Session.Branch }}`.
- Window actions can also use `{{ .Window.ID }}`.
- Declared inputs use `{{ .Inputs.<name> }}`.

Use the `shq` template function when inserting item or input data into a shell command.

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
