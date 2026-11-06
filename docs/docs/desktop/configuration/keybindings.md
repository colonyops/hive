---
icon: lucide/keyboard
description: Find, use, and change keyboard shortcuts in Hive Desktop.
---

# Keyboard shortcuts

Press <kbd>?</kbd> to see the current shortcuts. **Settings ▸ Keyboard** shows the same list, reports conflicts, and lets you change each binding.

The command palette at <kbd>⌘K</kbd> can run commands with or without a shortcut.

## Common shortcuts

| Action | Shortcut |
| --- | --- |
| Open command palette | <kbd>⌘K</kbd> |
| Open shortcut list | <kbd>?</kbd> |
| Search Drive documents | <kbd>⌘P</kbd> (Windows/Linux: <kbd>Ctrl+P</kbd>, outside a terminal) |
| Search the current view | <kbd>/</kbd> or <kbd>⌘F</kbd> |
| Find in a Code terminal | <kbd>⌘F</kbd> (Linux: <kbd>Ctrl+Shift+F</kbd>) |
| Open Settings | <kbd>⌘,</kbd> or <kbd>g</kbd> then <kbd>s</kbd> |
| Show or hide the current sidebar | <kbd>⌘B</kbd> (Windows/Linux: <kbd>Ctrl+B</kbd>) |
| Show or hide the right panel | <kbd>⌘⌥B</kbd> (Windows/Linux: <kbd>Ctrl+Alt+B</kbd>) |
| Show or hide a document's comments | <kbd>⌘⇧M</kbd> (Windows/Linux: <kbd>Ctrl+Shift+M</kbd>) |
| Go to Inbox | <kbd>g</kbd> then <kbd>i</kbd> |
| Go to Code | <kbd>g</kbd> then <kbd>c</kbd> |
| Go to Chats | <kbd>g</kbd> then <kbd>a</kbd> |
| Show or hide action runs | <kbd>g</kbd> then <kbd>l</kbd> |
| Refresh a feed | <kbd>r</kbd> |
| Move through feed items | <kbd>j</kbd>/<kbd>k</kbd> |
| Open an item | <kbd>o</kbd> or <kbd>Enter</kbd> |
| Archive an item | <kbd>e</kbd> |
| Increase or decrease text size of the area you last clicked | <kbd>⌘+</kbd>/<kbd>⌘-</kbd> (Windows/Linux: <kbd>Ctrl+=</kbd>/<kbd>Ctrl+-</kbd>; from a terminal: <kbd>Ctrl+Shift+=</kbd>/<kbd>Ctrl+Shift+-</kbd>) |
| Reset text size of the area you last clicked | <kbd>⌘0</kbd> (Windows/Linux: <kbd>Ctrl+0</kbd>; from a terminal: <kbd>Ctrl+Shift+0</kbd>) |

Use <kbd>?</kbd> for the full list because it always matches the installed version.

## Rebinding

You can change shortcuts in the app or in `settings.yaml`:

```yaml
keybindings:
  feed.next: [j, arrowdown]
  launcher.lazygit: [alt+g]
  tasks.toggle: ["g t"]
  palette.toggle: []
```

The map only needs entries you want to change. An empty list removes a binding.

Use lower-case key names and join modifiers with `+`, such as `mod+shift+t`. `mod` means Command on macOS and Control on other platforms. Separate keys with a space for a sequence such as `g i`.

## Applets

Applets have no shortcuts until you add them. They appear in **Settings ▸ Keyboard**, or you can name them in `settings.yaml`:

```yaml
keybindings:
  applet.docker: [mod+shift+d]
  applet.radio.next: [alt+n]
```

`applet.<id>` opens the applet where its file says. If it [lists a tab](applets.md#where-it-shows) and a session is on screen, that opens the tab or brings it forward. Otherwise it opens and closes the pop-up. `applet.<id>.<command>` runs a [command the applet lists](applets.md#palette-commands). Both also work from a focused terminal, whatever keys they use, so the terminal stops receiving that key. Avoid one your shell needs. With focus inside an applet page, a shortcut needs <kbd>⌘</kbd>, <kbd>Ctrl</kbd> or <kbd>⌥</kbd>, because the page keeps other keys.

While an applet's pop-up is open, only <kbd>⌘K</kbd>, the key that closes it, and that applet's commands run. A page can keep a shortcut for itself, so one it uses may not reach Hive.

## Session tabs

**Open Drive tab** (`session.open-drive`), **Open Diff tab** (`session.open-diff`), and **Open Tasks tab** (`session.open-tasks`) open that tab in the session on screen, in Code or in an open chat, or bring it forward. They have no default key. In the palette they appear under the session's name, only for the tabs that session offers: a chat has Drive but no Diff or Tasks. **Toggle Drive** and **Toggle Tasks** are different commands: they bring the app-wide Drive and Tasks tabs in the title bar forward, adding the tab if it is not there, and return to the tab you were on when pressed again.

```yaml
keybindings:
  session.open-diff: [mod+alt+d]
```

## Terminal shortcuts

A focused terminal sends most keys to the running program. A shortcut works from a terminal when it starts with Command on macOS, or with Control on Linux, where you press it with Control+Shift from inside the terminal. Shortcuts without one of those, such as <kbd>g</kbd> then <kbd>i</kbd>, only work outside a terminal, so typing `git` always reaches the shell.

This applies to shortcuts you add yourself. To switch to the inbox from a terminal, add a binding such as <kbd>⌘I</kbd> or <kbd>⌘G</kbd> then <kbd>i</kbd> to **Go to Inbox**. In **Settings ▸ Keyboard**, a terminal icon next to a shortcut shows that it works from a terminal and how to press it there.

On Linux, a shortcut whose first key already uses Shift, such as `mod+shift+b`, cannot be reached from a terminal.

**Find** (`terminal.find`) opens the search bar over a Code terminal. Over the session list and other views, <kbd>⌘F</kbd> searches the current view instead.

**Text size** (`view.text-size-increase`, `-decrease`, `-reset`) steps the area you last clicked or focused through 75, 80, 90, 100, 110, 120, 130, 150, 175, and 200%, within that area's range: a terminal, a Drive document, a diff or code field, or otherwise the interface. `interface.`, `document.`, `code.`, and `terminal.text-size-*` act on one area wherever you are and have no default key. The sizes are in **Settings ▸ Appearance ▸ Text size**. On macOS these chords do not zoom the window, and **Zoom In**, **Zoom Out** and **Actual Size** are gone from the **View** menu, because window zoom scales the window instead of reflowing the terminal grid.

**Toggle sidebar** (`terminal.toggle-sidebar`) shows or hides the sidebar in Inbox, Code, Chats, and the Drive page's folder tree. It uses <kbd>⌘B</kbd> on macOS and <kbd>Ctrl+B</kbd> on Windows and Linux, including inside a focused terminal. On macOS, <kbd>Ctrl+B</kbd> still reaches the terminal. On Windows and Linux, rebind this command in **Settings ▸ Keyboard** if you need <kbd>Ctrl+B</kbd> for tmux.

**Toggle right panel** (`view.toggle-panel`) is the same for the right edge, like the title bar's right button: the preview in Inbox and the comments column on the Drive page. Code and Chats have no right panel; Drive is a tab in the session there. **Toggle comments** (`drive.toggle-comments`) shows or hides the comments column on the Drive page.

Check **Settings ▸ Keyboard** before assigning a Control shortcut that your shell or terminal program already uses.
