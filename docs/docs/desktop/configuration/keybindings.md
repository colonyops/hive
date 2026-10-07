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
| Search the current view | <kbd>/</kbd> or <kbd>⌘F</kbd> |
| Find in a Code terminal | <kbd>⌘F</kbd> (Linux: <kbd>Ctrl+Shift+F</kbd>) |
| Open Settings | <kbd>⌘,</kbd> or <kbd>g</kbd> then <kbd>s</kbd> |
| Show or hide the current sidebar | <kbd>⌘B</kbd> (Windows/Linux: <kbd>Ctrl+B</kbd>) |
| Go to Inbox | <kbd>g</kbd> then <kbd>i</kbd> |
| Go to Code | <kbd>g</kbd> then <kbd>c</kbd> |
| Go to Chats | <kbd>g</kbd> then <kbd>a</kbd> |
| Show or hide action runs | <kbd>g</kbd> then <kbd>l</kbd> |
| Refresh a feed | <kbd>r</kbd> |
| Move through feed items | <kbd>j</kbd>/<kbd>k</kbd> |
| Open an item | <kbd>o</kbd> or <kbd>Enter</kbd> |
| Archive an item | <kbd>e</kbd> |
| Increase or decrease terminal text size | <kbd>⌘+</kbd>/<kbd>⌘-</kbd> (Windows/Linux, from a terminal: <kbd>Ctrl+Shift+=</kbd>/<kbd>Ctrl+Shift+-</kbd>) |
| Reset terminal text size | <kbd>⌘0</kbd> (Windows/Linux, from a terminal: <kbd>Ctrl+Shift+0</kbd>) |

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

## Terminal shortcuts

A focused terminal sends most keys to the running program. A shortcut works from a terminal when it starts with Command on macOS, or with Control on Linux, where you press it with Control+Shift from inside the terminal. Shortcuts without one of those, such as <kbd>g</kbd> then <kbd>i</kbd>, only work outside a terminal, so typing `git` always reaches the shell.

This applies to shortcuts you add yourself. To switch to the inbox from a terminal, add a binding such as <kbd>⌘I</kbd> or <kbd>⌘G</kbd> then <kbd>i</kbd> to **Go to Inbox**. In **Settings ▸ Keyboard**, a terminal icon next to a shortcut shows that it works from a terminal and how to press it there.

On Linux, a shortcut whose first key already uses Shift, such as `mod+shift+b`, cannot be reached from a terminal.

**Find** (`terminal.find`) opens the search bar over a Code terminal. Over the session list and other views, <kbd>⌘F</kbd> searches the current view instead.

**Text size** (`terminal.text-size-increase`, `-decrease`, `-reset`) steps the terminal font by 2px a press, between 8px and 64px, wherever a terminal is on screen (a Code session, Chats, or the pop-up panel) and saves the result to the same setting. On macOS these chords no longer zoom the window, and **Zoom In**, **Zoom Out** and **Actual Size** are gone from the **View** menu, because window zoom scales the window instead of reflowing the terminal grid.

**Toggle sidebar** (`terminal.toggle-sidebar`) shows or hides the sidebar in Inbox, Code, and Chats. It uses <kbd>⌘B</kbd> on macOS and <kbd>Ctrl+B</kbd> on Windows and Linux, including inside a focused terminal. On macOS, <kbd>Ctrl+B</kbd> still reaches the terminal. On Windows and Linux, rebind this command in **Settings ▸ Keyboard** if you need <kbd>Ctrl+B</kbd> for tmux.

Check **Settings ▸ Keyboard** before assigning a Control shortcut that your shell or terminal program already uses.
