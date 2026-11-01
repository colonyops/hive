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
- Bind one to a key in **Settings ▸ Keyboard**, or in `settings.yaml` as `applet.<id>`. The same key closes it again.

<kbd>esc</kbd> closes an applet, unless the page uses the key itself first.

## Palette and keys

An applet can add its own rows to the command palette at <kbd>⌘K</kbd>:

- Commands its `applet.yaml` lists are always there. You can bind each one in **Settings ▸ Keyboard**, or in `settings.yaml` as `applet.<id>.<command>`. They have no shortcut until you give them one.
- Rows a page adds while it runs, such as a list of stations, last as long as that page and cannot be bound.

Shortcuts keep working with focus inside a page, as long as they use <kbd>⌘</kbd>, <kbd>Ctrl</kbd> or <kbd>⌥</kbd>. While a panel is open, only <kbd>⌘K</kbd>, that applet's own key, and that applet's commands run. A page can keep a shortcut for itself. See [Keyboard shortcuts](keybindings.md#applets).

## Turn one off

Each applet has a switch in **Settings ▸ Applets**. An applet that is off stays listed and keeps its files, saved data, title bar place and shortcuts, but it does not run, serve its pages or appear in the palette. Turning it off closes what it has open and stops everything it has running. In `settings.yaml`, where a change to the list closes open pages at the next launch:

```yaml
applets:
  disabled: [radio]
```

## Put one in the title bar

An applet can offer a page for the title bar, such as an indicator, music controls, or a shortcut to another part of the app. Ask for one when you ask the agent for the applet.

Turn it on in **Settings ▸ Applets** with its **Title bar** switch. The page then loads with the app and keeps running until you turn it off.

Hive never adds one for you: a new applet stays out of the bar until you turn it on. To set the order, list the ids in `settings.yaml`:

```yaml
applets:
  titlebar: [music, build-status]
```

## Share one

Export and import are in **Settings ▸ Applets**.

!!! warning "Only import applets from people you trust"
    An applet has full access to your machine, as you. Hive does not review or limit it. Once you open it, it can run any command or program, keep processes running, read, change and delete any of your files, reach the network and send what it reads anywhere, use your clipboard, ask for your camera and microphone, and run Hive's commands. Importing one is the same as running a program someone sent you.

**To export**, press **Export** on the applet, check the list of files, and save. You get one `.hiveapplet` file.

| In the file | Left out |
| --- | --- |
| The applet's own files, in their folders | Saved `localStorage` |
| | Hidden files and folders such as `.env` and `.git` |
| | `.log` files |
| | Links, so nothing outside the applet's folder can get in |

The file holds no settings, shortcuts or other data from your machine. Hive cannot tell every file a page wrote from one you wrote, so read the list, and anyone you send the file to can read all of it. Built-in applets have no **Export**.

**To import**, press **Import** and choose the file. Hive shows the warning, the applet's name, its files, and what would run. Nothing is on disk until you tick **I trust who this came from** and press **Import applet**. Cancel is the default and leaves nothing behind.

An import never replaces an applet. If the id is taken, Hive offers a new one. To update an applet, delete its folder first, which also deletes its saved data. An imported applet starts with nothing:

- Nothing runs until you open it.
- A title bar page stays off until you turn it on, even if that id was in your title bar before.
- It has no shortcuts, even if that id had some before.
- Files are not marked executable, so a page has to run a script with `sh script.sh`.

Hive refuses a file that is over 32 MB, unpacks past 64 MB, or has more than 500 files. It also refuses one with paths outside the applet, links, hidden files, or an invalid `applet.yaml`, and installs none of it.

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
| `hive.commands()`, `hive.command(id)` | Lists Hive's commands with `{id, title, group, keys}`, and runs one. An unknown id rejects |
| `hive.setCommands(rows)` | Replaces the palette rows this page added |
| `hive.on('command', fn)` | Reports the id of the applet's command or row that was run |
| `hive.openPalette(query)` | Opens the command palette on a query. `+`, `@` or `>` first picks a tab |
| `hive.navigate(path)` | Goes to a route of the app, such as `/settings/applets` |
| `hive.broadcast(data)`, `hive.on('message', fn)` | Talks to the applet's other open frames |
| `hive.on('session', fn)` | Reports the session on screen when it changes |
| `hive.surface` | `page`, `titlebar`, or `dropdown` |

The page also receives Hive's theme as CSS variables such as `--hv-app`, `--hv-text` and `--hv-accent`, which change when you change theme. `localStorage` works and is kept per applet. Cookies and IndexedDB do not work.

!!! warning "An applet can do anything you can"
    Applets have no permissions and no allowlist. A page can run any command and change any file, and so can any remote script it loads. Nothing runs when a file loads, only when you open an applet. Only [import applets](#share-one) from people you trust.

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

A dropdown loads again each time it opens, so put anything that has to keep going, such as audio, in the title bar page and talk to it with `hive.broadcast`. `localStorage` is shared between an applet's frames. Closing a panel or dropdown stops only the `hive.spawn` processes it started. A `hive.exec` already running finishes or times out.

A small title bar page looks like this:

```html
<!doctype html>
<style>
  html, body { margin: 0; height: 100%; background: transparent; }
  button { width: 100%; height: 28px; border: 0; border-radius: 8px; background: none; color: var(--hv-text-2, #aaa); }
  button:hover { background: var(--hv-chip, #333); }
</style>
<button id="count">0</button>
<script>
  const button = document.querySelector('#count')
  const refresh = async () => {
    const { stdout } = await hive.exec('docker ps -q | wc -l')
    button.textContent = stdout.trim()
  }
  button.onclick = () => hive.open()
  refresh()
  setInterval(refresh, 10000)
</script>
```

### Palette commands

List commands in `applet.yaml` to give the palette rows that a user can bind to a key. `id` is a slug and becomes part of the binding, `applet.<applet id>.<id>`, so keep it once people use it. `icon` and `keywords` are optional. The palette shows `title` without the applet's name in some views, so make it make sense alone.

```yaml
commands:
  - id: next
    title: Next radio station
    icon: play
    keywords: [skip]
```

Listen for them in the page:

```js
hive.on('command', (id) => {
  if (id === 'next') playNext()
})
```

A command goes to the applet's running persistent page, which today means the title bar page if it is on. Otherwise the panel opens and the page there gets it, even if it starts listening after it loads. To have a command open a dropdown, call `hive.dropdown()` from the listener in the title bar page.

For rows that change, such as things only a running page knows about, call `hive.setCommands`. Each call replaces the rows that page added before, and they disappear when the page closes or reloads. They arrive through the same `hive.on('command')`. Only `id` and `title` are required.

```js
hive.setCommands(stations.map((station) => ({
  id: station.id,
  title: station.name,
  group: 'Stations',
  scope: 'goto',
  kind: 'station',
  hint: station.genre,
  keywords: [station.genre],
  icon: 'play',
})))
```

`group` defaults to the applet's name, and `scope` to `actions`. `kind` is the small word at the row's right edge, for a row that names a thing rather than an action.

### Keys in a page

Command, Control and Option shortcuts go to the page first, then to Hive unless the page calls `preventDefault()`. Keep a shortcut for the page like this:

```js
addEventListener('keydown', (event) => {
  if (event.metaKey && event.key === 'k') {
    event.preventDefault()
    openMySearch()
  }
})
```

Keys with no modifier stay in the page, apart from <kbd>esc</kbd>, so typing never runs a Hive shortcut. The exception is the rest of a sequence such as <kbd>⌘G</kbd> then <kbd>i</kbd>, which goes to Hive once the first step has started it.

If a file is invalid, the palette marks the applet **(invalid)** and opening it shows the error. An applet that loaded before keeps running its last valid version.

### Keep an applet working across releases

`version` in `applet.yaml` is the format of the file, and stays `1`. It is not a release number for your applet, and a future `release:` key would hold that.

A key this Hive does not know, or an icon it does not have, is a warning: the applet opens without it, and **Settings ▸ Applets** and the palette say so. A wrong value under a key Hive knows is an error. A file with a later `version` asks you to update Hive.

To need a later Hive, add `hive`. An older one then asks you to update instead of running the file:

```yaml
version: 1
hive: ">=0.60"
title: Music
```

Always set `hive` when a key limits what the applet may do, because an older Hive would otherwise ignore the limit and run the applet without it.

Command ids and routes that `hive.command`, `hive.commands` and `hive.navigate` reach are part of this API. If Hive renames one, the old name keeps working.

`close` and `resize` act on the frame that calls them, and mean what fits its surface. `open` and `dropdown` act on another surface. New verbs follow the same rule.

To replace the Applet Builder with your own, create a directory named `applet-builder`.
