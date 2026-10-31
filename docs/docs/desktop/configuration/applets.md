---
icon: lucide/layout-panel-top
description: Ask an agent for a web page that opens over Hive Desktop or sits in its title bar and can do anything on your machine, with the Applet Builder to get you started.
---

# Applets

An applet is a web page that opens over the app. It can be a Docker viewer, a Git graph, a dashboard, a game, or whatever else you want, and it can run any command on your machine.

## Make one

Open **Applet Builder** from the command palette at <kbd>⌘K</kbd>. Pick a style (Hive, a website, a design style, or something it should feel like), then copy the prompt.

Paste the prompt to an agent. The agent asks what the applet should do, then writes it.

In a chat, you can also just ask. Workspaces with the `hive` skill package carry the `hive-applets` skill.

The applet appears as soon as its file is valid. If you dislike it, ask for changes or delete its directory.

## Open one

- Press <kbd>⌘K</kbd> and type its name. An applet whose name matches comes first, under **Applets**.
- Press <kbd>⌘K</kbd> and type <kbd>+</kbd>, or press <kbd>⇥</kbd> to reach the **Applets** tab, to see only applets.
- Bind one to a key in **Settings ▸ Keyboard**, or in `settings.yaml` as `applet.<id>`.

<kbd>esc</kbd> closes an applet, unless the page uses the key itself first.

## Put one in the title bar

An applet can offer a page for the title bar, such as an indicator, music controls, or a shortcut to another part of the app. Ask for one when you ask the agent for the applet.

Turn it on in **Settings ▸ Applets** with its **Title bar** switch. The page then loads with the app and keeps running until you turn it off.

Hive never adds one for you: a new applet stays out of the bar until you turn it on. To set the order, list the ids in `settings.yaml`:

```yaml
applets:
  titlebar: [music, build-status]
```

## See what is registered

**Settings ▸ Applets** lists every applet Hive found and whether its file loaded, with the error if not. It also shows what applets have running, so you can stop a process, and where the applets folder is.

## The files

An applet is one directory under `~/.config/hive/desktop/applets/`, named for its id, holding an `applet.yaml` and its page.

```yaml
version: 1
title: Containers
icon: container
width: 1200
height: 760
```

Put an `index.html` beside it. Scripts, modules, images, audio, and network requests all work, and editing a file reloads the page.

The page gets `window.hive`:

| Call | What it does |
| --- | --- |
| `hive.exec(command, options)` | Runs a command and returns `{stdout, stderr, code}` |
| `hive.spawn(command, options)` | Starts a long-running process with `onStdout`, `onStderr`, and `onExit` callbacks, and returns `{write, kill}` |
| `hive.readFile(path)`, `hive.writeFile(path, text)` | Reads and writes files anywhere |
| `hive.terminal(command)` | Opens the pop-up terminal |
| `hive.toast(message, options)` | Shows a toast |
| `hive.copy(text)` | Puts text on the clipboard |
| `hive.context()` | Returns the applet's directory, working directory, and session |
| `hive.close()` | Closes the applet. In a title bar page, closes its dropdown |
| `hive.open()` | Opens the applet's panel |
| `hive.commands()`, `hive.command(id)` | Lists Hive's commands, and runs one |
| `hive.navigate(path)` | Goes to a route of the app |
| `hive.broadcast(data)`, `hive.on('message', fn)` | Talks to the applet's other open frames |
| `hive.on('session', fn)` | Reports the session on screen when it changes |
| `hive.surface` | `page`, `titlebar`, or `dropdown` |

The page also receives Hive's theme as CSS variables such as `--hv-app`, `--hv-text` and `--hv-accent`, which change when you change theme. `localStorage` works and is kept per applet. Cookies and IndexedDB do not work.

!!! warning "An applet can do anything you can"
    Applets have no permissions and no allowlist. A page can run any command and change any file, and so can any remote script it loads. Nothing runs when a file loads, only when you open an applet. Read an applet from someone else before you open it.

### Title bar pages

Add `titlebar` to `applet.yaml`. `entry` defaults to the applet's own page, and `width` to 28, one icon button.

```yaml
version: 1
title: Music
titlebar:
  entry: bar.html
  width: 120
```

The slot is 28px tall, and a page with a transparent background sits directly on the bar. Two more calls apply:

| Call | What it does |
| --- | --- |
| `hive.resize({width, height})` | Resizes the slot or dropdown. Width `0` hides the slot and keeps the page running |
| `hive.dropdown({entry, width, height})` | Opens a panel under the bar, or closes it if it is open |

A dropdown loads again each time it opens, so put anything that has to keep going, such as audio, in the title bar page and talk to it with `hive.broadcast`. `localStorage` is shared between an applet's frames. Closing a panel or dropdown stops only the processes it started.

If a file is invalid, the palette marks the applet **(invalid)** and opening it shows the error. An applet that loaded before keeps running its last valid version.

To replace the Applet Builder with your own, create a directory named `applet-builder`.
