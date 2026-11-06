---
icon: lucide/layout-panel-top
description: Ask an agent for a web page that says where it belongs, as a session tab, a pop-up over Hive Desktop, or a page in its title bar, and can do anything on your machine, with the Applet Builder to get you started.
---

# Applets

!!! warning "Experimental"
    Applets are experimental. How they work, and the `applet.yaml` format, may change.

An applet is a web page that opens as a session tab or a pop-up. It can be a Docker viewer, a Git graph, a dashboard, a game, or whatever else you want, and it can run any command on your machine.

## Make one

Open **Applet Builder** from the command palette at <kbd>⌘K</kbd>. Pick a style (Hive, a website, a design style, or something it should feel like), then copy the prompt.

Paste the prompt to an agent. The agent asks what the applet should do, then writes it.

In a chat, you can also just ask. Workspaces with the `hive` skill package carry the `hive-applets` skill.

The applet appears as soon as its file is valid. If you dislike it, ask for changes or [delete it](#delete-one).

## Open one

- Press <kbd>⌘K</kbd> and type its name. An applet whose name matches comes first, under **Applets**.
- Press <kbd>⌘K</kbd> and type <kbd>+</kbd>, or press <kbd>⇥</kbd> to reach the **Applets** tab, to see only applets.
- Bind one to a key in **Settings ▸ Keyboard**, or in `settings.yaml` as `applet.<id>`. The same key closes a pop-up again.

To keep one at hand, press **+** in the title bar and pick it under **Applets**. It opens as a tab of its own beside Inbox, Code, and Chats, stays in the title bar across restarts, and keeps running behind other tabs once you have opened it. Any applet that lists a `tab` or a `popup` can be a title-bar tab, with no change to its file. Once it is there, opening the applet from the palette or its key brings that tab forward.

Otherwise it opens where [its file says](#where-it-shows). An applet with a tab opens as that tab when a session is on screen. Otherwise it opens as a pop-up, on the session on screen. An applet with only a tab asks you to open a session first.

<kbd>esc</kbd> closes a pop-up, unless the page uses the key itself first. A tab stays open; close it from its tab.

## Where it shows

An applet's `applet.yaml` says where it belongs, under `surfaces`. It shows in each one listed and nowhere else. With none listed, it is a pop-up.

```yaml
version: 1
title: Containers
icon: container
surfaces:
  tab: {}
  popup:
    width: 1200
    height: 760
  titlebar:
    entry: bar.html
    width: 64
```

| Surface | Where | Settings | Runs |
| --- | --- | --- | --- |
| `tab` | In a session's **+** menu, on a tab row and on a session in the sidebar, for chats as well as Code sessions | None | From opening the tab until you close it or switch to another session |
| `popup` | Over the app, from the palette or a key | `width` and `height` in pixels. Unset fills the window | While it is open |
| `titlebar` | In the title bar | `entry`, default the applet's own page, and `width` in pixels, default 28 | From the moment the file is valid, whenever Hive is open |

A tab sits beside the session's terminals and Drive, and you can drag it between panels. Its page keeps running behind other tabs, through a move, and while you are in another part of the app. A session keeps its applet tabs across restarts. If you turn an applet off, or its file stops listing a tab, the tab stays and says so until you close it. Deleting an applet closes its tabs.

The same shortcut brings a tab forward, and closes a pop-up.

To change where an applet shows, select it in **Settings ▸ Applets** and use the switches, which save to its `applet.yaml`. Turning a surface off writes `enabled: false`, so it keeps its size and page for when you turn it back on. You can also edit the file. A title bar needs a page, so that switch stays off until the file lists `surfaces.titlebar`. The Applet Builder ships with Hive and has no file, so where it shows is fixed.

Agents can open and close a tab too, with [`tab_open` and `tab_close`](../code/agent-layout.md).

## Palette and keys

An applet can add its own rows to the command palette at <kbd>⌘K</kbd>:

- Commands its `applet.yaml` lists are always there. You can bind each one in **Settings ▸ Keyboard**, or in `settings.yaml` as `applet.<id>.<command>`. They have no shortcut until you give them one.
- Rows a page adds while it runs, such as a list of stations, show while that page is on screen, last as long as it does, and cannot be bound.

Shortcuts keep working with focus inside a page. See [Keyboard shortcuts](keybindings.md#applets).

## Turn one off

Each applet has a switch in **Settings ▸ Applets**. An applet that is off stays listed and keeps its files, saved data and shortcuts, but it does not run, serve its pages or appear in the palette. Turning it off closes what it has open and stops everything it has running. In `settings.yaml`, where a change to the list closes open pages at the next launch:

```yaml
applets:
  disabled: [radio]
```

## Delete one

In **Settings ▸ Applets**, select the applet, press **Delete applet**, and confirm. This stops everything it has running and removes its folder, saved data, session tabs and shortcuts. It cannot be undone, so [export it](#share-one) first if you might want it back.

A built-in applet has no delete, so turn it off instead. Deleting a folder that [replaced the Applet Builder](#keep-an-applet-working-across-releases) brings the built-in back.

## Share one

Export and import are in **Settings ▸ Applets**.

!!! warning "Only import applets from people you trust"
    An applet has full access to your machine, as you. Hive does not review or limit it. Once you open it, it can run any command or program, keep processes running, read, change and delete any of your files, reach the network and send what it reads anywhere, use your clipboard, ask for your camera and microphone, and run Hive's commands. Importing one is the same as running a program someone sent you.

**To export**, select the applet, press **Export**, check the list of files, and save. You get one `.hiveapplet` file.

| In the file | Left out |
| --- | --- |
| The applet's own files, in their folders | Saved `localStorage` |
| | Hidden files and folders such as `.env` and `.git` |
| | `.log` files |
| | Links, so nothing outside the applet's folder can get in |

The file holds no settings, shortcuts or other data from your machine. Hive cannot tell every file a page wrote from one you wrote, so read the list, and anyone you send the file to can read all of it. Built-in applets have no **Export**.

**To import**, press **Import** and choose the file. Hive shows the warning, the applet's name, its files, and what would run, including a title bar page, which starts as soon as you import. Nothing is on disk until you tick **I trust who this came from** and press **Import applet**. Cancel is the default and leaves nothing behind.

An import never replaces an applet. If the id is taken, Hive offers a new one. To update an applet, [delete it](#delete-one) first, which also deletes its saved data. An imported applet shows where its file says, and inherits nothing from an earlier applet with that id:

- A title bar page starts right away. Anything else runs when you open it.
- It has no open tabs, even if that id did before.
- It has no shortcuts, and is not off, even if that id was before.
- Files are not marked executable, so a page has to run a script with `sh script.sh`.

Hive refuses a file that is over 32 MB, unpacks past 64 MB, or has more than 500 files. It also refuses one with paths outside the applet, links, hidden files, or an invalid `applet.yaml`, and installs none of it.

## See what is registered

**Settings ▸ Applets** lists every applet Hive found and whether its file loaded, with the error if not. It also shows what applets have running, so you can stop a process, and where the applets folder is. Select an applet to see where it lives, what it has running, its commands and its files.

## The files

An applet is one directory under `~/.config/hive/desktop/applets/`, named for its id, holding an `applet.yaml` and its page. Two lines are enough for a pop-up that fills the window, and [`surfaces`](#where-it-shows) says where else it goes.

```yaml
version: 1
title: Containers
```

Put an `index.html` beside it. Scripts, modules, images, audio, and network requests all work, and editing a page, script or stylesheet reloads the page.

The page gets `window.hive`:

| Call | What it does |
| --- | --- |
| `hive.exec(command, options)` | Runs a command and returns `{stdout, stderr, code}`. Commands get the environment Hive read from your login shell at startup. If that read failed, they get the app's own `PATH`, and the app log says so |
| `hive.spawn(command, options)` | Starts a long-running process with `onStdout`, `onStderr`, and `onExit` callbacks, and returns `{write, kill}` |
| `hive.readFile(path)`, `hive.writeFile(path, text)` | Reads and writes files anywhere. `writeFile` creates missing parent directories |
| `hive.terminal(command)` | Opens the pop-up terminal |
| `hive.toast(message, options)` | Shows a toast |
| `hive.copy(text)` | Puts text on the clipboard |
| `hive.startChat({workspace, name, prompt, pinToCode})` | Starts an agent workspace chat in the background, optionally pinned to Code, and returns `{id, workspace, name, slug, pinnedAt}`. The prompt needs a workspace command that passes `.Prompt` |
| `hive.pinChat(id)`, `hive.unpinChat(id)` | Pins a chat to the Code sidebar or unpins it |
| `hive.context()` | Returns the applet's directory, working directory, and session |
| `hive.close()` | Closes the applet's pop-up or tab, including its title-bar tab. In a title bar page, closes its dropdown |
| `hive.open()` | Opens the applet: as a tab if its file lists one and a session is on screen, otherwise as a pop-up if it lists one. Closes its pop-up if that is already open |
| `hive.commands()`, `hive.command(id)` | Lists Hive's commands with `{id, title, group, keys}`, and runs one. An unknown id rejects. A command takes no arguments: `session.new` opens an empty dialog |
| `hive.setCommands(rows)` | Replaces the palette rows this page added |
| `hive.on('command', fn)` | Reports the id of the applet's command or row that was run |
| `hive.openPalette(query)` | Opens the command palette on a query. `+`, `@` or `>` first picks a tab |
| `hive.navigate(path)` | Goes to a route of the app, such as `/settings/applets` |
| `hive.broadcast(data)`, `hive.on('message', fn)` | Talks to the applet's other open frames |
| `hive.on('session', fn)` | Reports the session on screen when it changes |
| `hive.surface` | `tab`, `page` (a tab of its own in the title bar), `popup`, `titlebar`, or `dropdown` |

The page also receives Hive's theme as CSS variables such as `--hv-app`, `--hv-text` and `--hv-accent`, which change when you change theme. `localStorage` works and is kept per applet. Cookies and IndexedDB do not work.

!!! warning "An applet can do anything you can"
    Applets have no permissions and no allowlist. A page can run any command and change any file, and so can any remote script it loads. A title bar page runs as soon as its file is valid. Anything else runs when you or an agent open it.

### Title bar pages

Use this for an indicator, music controls, or a shortcut to another part of the app. Add `titlebar` under `surfaces`. `entry` defaults to the applet's own page, and `width` to 28, one icon button. Title bar pages sit in order of applet id.

```yaml
version: 1
title: Music
surfaces:
  titlebar:
    entry: bar.html
    width: 120
```

An applet with only `titlebar` has no row to open it in the palette, and a page it has running takes its commands.

The slot is 28px tall, and a page with a transparent background sits directly on the bar. Two more calls apply:

| Call | What it does |
| --- | --- |
| `hive.resize({width, height})` | Resizes the slot or dropdown. Width `0` hides the slot and keeps the page running |
| `hive.dropdown({entry, width, height})` | Opens a dropdown under the bar, or closes it if it is open |

A dropdown loads again each time it opens, so put anything that has to keep going, such as audio, in the title bar page and talk to it with `hive.broadcast`. `localStorage` is shared between an applet's frames. Closing a pop-up, tab or dropdown stops only the `hive.spawn` processes it started. A `hive.exec` already running finishes or times out.

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

A command goes to the applet's running persistent page, which today means the title bar page. Otherwise it goes to the applet's page on screen, or the applet opens where its file says. The page there gets the command, even if it starts listening after it loads. To have a command open a dropdown, call `hive.dropdown()` from the listener in the title bar page.

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

`version` in `applet.yaml` is the format of the file, and stays `1`. It is not a release number for your applet.

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
