---
icon: lucide/layout-panel-top
description: Ask an agent for a table, a fuzzy picker, or a whole web page that opens over Hive Desktop and can do anything on your machine.
---

# Applets

An applet is a UI that opens over the app. It can be a table for watching and managing things, a fuzzy picker for choosing one thing and acting on it, or a web page of its own that does whatever you want.

## Make one

An agent writes the applet for you.

- **In a chat**, ask for what you want. Workspaces with the `hive` skill package carry the `hive-applets` skill.
- **Anywhere else**, run **Copy Applet Authoring Prompt** from the command palette at <kbd>⌘K</kbd>, paste it to an agent, and describe what you want.

Try "a Docker viewer where I can see my containers, read their logs, and restart them", "a picker that runs a mise task", or "Pong, where my busiest process is the opponent".

The applet appears as soon as its file is valid. If you dislike it, ask for changes or delete its directory.

## Open one

Applets are in the command palette under **Applets**. Bind one to a key in **Settings ▸ Keyboard**, or in `settings.yaml` as `applet.<id>`. An applet that runs in the current session appears only while a session is on screen.

| View | Keys |
| --- | --- |
| Table | <kbd>j</kbd> <kbd>k</kbd> move, <kbd>/</kbd> filters, <kbd>↵</kbd> runs the first action, each action's own key runs it, <kbd>esc</kbd> closes |
| Picker | Type to filter, <kbd>↑</kbd> <kbd>↓</kbd> move, <kbd>↵</kbd> runs the first action and closes |
| Page | Whatever the page does. <kbd>esc</kbd> closes unless the page keeps it |

## The file

An applet is one directory under `~/.config/hive/desktop/applets/`, named for its id, holding an `applet.yaml` and any scripts it calls.

```yaml
version: 1
title: Mise tasks
icon: play
view: picker
data:
  command: mise tasks --json
columns:
  - key: name
    label: Task
  - key: description
actions:
  - id: run
    label: Run
    mode: terminal
    run: mise run "$HIVE_ROW_NAME"
```

An action reads the selected row from `$HIVE_ROW_<KEY>`, or all of it as JSON from `$HIVE_ROW`. The `hive-applets` skill has the full schema.

## Your own page

Set `view: html` and put an `index.html` beside the file. Scripts, modules, images, audio, and network requests all work, and editing a file reloads the page.

```yaml
version: 1
title: Pong
view: html
```

The page gets `window.hive`:

| Call | What it does |
| --- | --- |
| `hive.exec(command, options)` | Runs a command and returns `{stdout, stderr, code}` |
| `hive.spawn(command, options)` | Starts a long-running process with `onStdout`, `onStderr`, and `onExit` callbacks, and returns `{write, kill}` |
| `hive.readFile(path)`, `hive.writeFile(path, text)` | Reads and writes files anywhere |
| `hive.terminal(command)` | Opens the pop-up terminal |
| `hive.toast(message, options)` | Shows a toast |
| `hive.context()` | Returns the applet's directory, working directory, and session |
| `hive.close()` | Closes the applet |

`localStorage` works and is kept per applet. Cookies and IndexedDB do not work.

!!! warning "An applet can do anything you can"
    Applets have no permissions and no allowlist. A page can run any command and change any file, and so can any remote script it loads. Nothing runs when a file loads, only when you open an applet. Read an applet from someone else before you open it.

If a file is invalid, the palette marks the applet **(invalid)** and opening it shows the error. An applet that loaded before keeps running its last valid version and shows the error above its rows.
