---
name: desktop-commands-audit
description: Check the work on this branch (or a PR) for anything in Hive Desktop that should be reachable from the command palette or bindable to a key and is not, then add the catalog entries and palette rows. Use when asked to audit keybindings, shortcuts, or the palette, to check what ⌘K is missing, to decide whether a feature deserves a chord, or as a pre-PR pass over a feature that added a view, an object, or a verb.
---

# Audit the palette and keymap

⌘K is the app's index, and a bindable command is a contract: its id is what a
user writes in `settings.yaml`'s `keybindings:` section, so it outlives every
rename of the code behind it. The failure mode of both is silent. A feature
ships, nobody registers a row or a command, and the palette quietly stops
being complete.

The doctrine is in `docs/architecture.md` (the keymap section and "The command
palette's rules sit beside the keymap doctrine") plus these ADRs. Read them
before deviating, not before every audit:

- [keybindings-are-chord-sequences-not-a-leader-key](../../../docs/decisions/2026-08-29-keybindings-are-chord-sequences-not-a-leader-key.md)
- [palette-scopes-are-filters-over-one-list](../../../docs/decisions/2026-08-29-palette-scopes-are-filters-over-one-list.md)
- [palette-rows-name-objects-and-rendering-draws-the-path](../../../docs/decisions/2026-08-29-palette-rows-name-objects-and-rendering-draws-the-path.md)
- [a-command-is-a-source](../../../docs/decisions/2026-08-05-a-command-is-a-source.md)

## 1. Scope the change

```bash
git diff main...HEAD --stat
git diff main...HEAD -- cmd/desktop/frontend/src
```

For a PR that is not checked out, `gh pr diff <number>`.

## 2. Inventory what a user gained

Write these down before deciding anything:

- **verbs** -- a new button, menu entry, context-menu item, toolbar icon, or
  anything with an `@click` a user can reach;
- **objects** -- a new kind of thing that can be listed and selected (a
  workspace, a canvas, a pinned chat, a window, a source);
- **places** -- a new view, overlay, dialog, mode, or settings section;
- **configured things** -- a new action type, launcher, or flow surface that a
  user's `actions.yml` can now name.

An internal refactor, a change to how an existing row is drawn, and a
backend-only change all inventory to nothing. Say so and stop. An audit that
finds nothing is a valid outcome and is reported as one.

## 3. Classify each one

**A stable app verb is a command.** Something a user does repeatedly, that
means the same thing every time, and that has one implementation: new views,
overlays, toggles, and lifecycle operations. It goes in the catalog (step 4),
and `useAppPaletteRows` seeds its palette row from there automatically.

**A verb that only exists where its object does is a row, not a command.**
Killing the attached session, unpinning this chat, deleting this workspace:
these act on state a component owns, so they register as `scope: 'actions'`
rows from that component and are not bindable. `TerminalMode.vue`'s
`terminal:session:kill` is the pattern. Group them under the object's own
name.

**An object is data, not a command.** Feeds, profiles, sessions, windows,
chats, workspaces, themes, settings sections: register these with
`useCommands` as a reactive source (step 6). They are never catalog entries.

**These are neither, and saying so is half the audit:**

- **Widget-local navigation.** The session tree's arrows and `j`/`k`, the
  palette's own arrows. A combo resolves to exactly one command (the first in
  the catalog claiming it), so `context` narrows *when* a command fires but
  never lets two commands share a chord. Those stay handlers on the widget
  that owns focus.
- **Anything reached once per session** from a settings page or a dialog.
- **A launcher or a configured action.** Each `launchers:` entry in
  `actions.yml` already contributes `launcher.<id>` through
  `setLauncherCommands`, unbound by default, and actions reach the palette
  through the selected item's own rows. A new action *type* needs nothing;
  check it flows through those paths rather than building a parallel one.
- **A numbered variant.** `terminal.select-window-1` … `-9` are generated. A
  tenth is deliberately unreachable.

## 4. Add a command to the catalog

`cmd/desktop/frontend/src/keybindings/catalog.ts`, in `commandCatalog`:

```ts
{
  id: 'notes.toggle',        // <area>.<verb>, dot-separated, permanent
  title: 'Toggle Notes',     // the exact string Settings ▸ Keyboard shows
  group: 'View',             // shared with the palette's grouping
  keywords: ['notes', 'scratch'],
  icon: IconNotebook,
  defaultCombos: [],         // canonical combos; [] = bindable, unbound
  context: 'global',
}
```

`tasks.toggle` in the same file is the worked version, with a default chord,
a sequence, `piercesPane`, and a comment saying why.

- **`id` is config.** `<area>.<verb>`, lower-case, dots. It is written into a
  user's `settings.yaml`, so choose it once and never rename it to match a
  refactor.
- **`context` gates where a bare, modifier-less binding fires**: `global`,
  `feed`, `terminal`, `terminal-session`, `agents`. `contextActive` in
  `App.vue` enforces it; a new context means teaching that switch what
  "active" means for it.
- **Combos are canonical strings, not display glyphs.** `mod+shift+t`,
  `alt+t`, `g t`: lower-case, modifiers ordered by `canonicalizeCombo`,
  sequence steps space-joined. `mod` is Command on macOS and Ctrl elsewhere,
  so check a `mod` chord against readline on Linux. `formatCombo` makes the
  ⌘-and-⇧ form; never write that form into the catalog.
- **`defaultCombos: []` is the right default.** Ship a default only for a
  platform standard (`⌘,`, `⌘N`, `⌘[`/`⌘]`) or a command used often enough to
  earn it. A destructive command with no undo stays unbound on purpose
  (`feed.mark-workspace-read`).
- **Check the combo is free before you claim it.** Catalog order decides
  ties, so an added entry can silently shadow a later one. Launchers are
  appended after the catalog so a config file cannot take `mod+k` from the
  palette.
- Add `scope: 'goto'` when the command navigates rather than acts, and
  `paletteHidden: true` only when a named dynamic row already stands in for
  it (as `view.focus-search` does per surface).

### Wire the implementation

One entry in `App.vue`'s `runMap`, keyed by the id:

```ts
'tasks.toggle': openTasks,
```

The keydown dispatcher and the palette both run through that map. Launchers
and the numbered window jumps are the exceptions, resolved ahead of the map in
`runCommand`.

**A catalog entry with no `runMap` entry fails silently.** The row appears in
the palette, the shortcut appears in Settings, and the key does nothing.
Check the pair by hand.

### A focused terminal pane

A pane keeps every key it can use. Two flags take one back:

- **`escapesPane`**, claimed through `terminalEscapeCombo`: Command chords,
  and Ctrl+Shift where there is no Command. **Prefer this.** It leaves bare
  Ctrl+K as readline's kill-to-end-of-line while ⌘K is the palette's.
- **`piercesPane`**, claimed on the binding alone, whatever modifiers it
  carries. Only for a chord the escape form cannot express (an `alt`
  binding), a combo that must *close* what it opened
  (`terminal.popup.toggle`, `tasks.toggle`), or the half of a focus pair that
  reaches back out of a pane.

Both are two-sided: the dispatcher acts on the chord **and** xterm's
`attachCustomKeyEventHandler` declines it, or the pane writes the key to tmux
as well. If you cannot answer "why may a pane not have this key?", set
neither flag.

### Sequences

A binding is one combo or a space-separated sequence. Navigation to a place
uses `g <x>` (`g i`, `g c`, `g a`, `g t`, `g s`). A sequence start never fires
over a focused pane, into an editable target, or under an overlay, and
`resolve` stays single-step. Adding a `g <x>` means checking nothing else
binds bare `x` in an overlapping context.

## 5. Check the command's palette row

The catalog seeds the row, so confirm it appears: it is not `paletteHidden`,
its `context` is active wherever the verb makes sense, and its title is the
catalog title unchanged, since Settings ▸ Keyboard reads the same string.

## 6. Register object and action rows

```ts
useCommands(computed(() => workspaces.value.map((w) => ({
  id: `agents:workspace:${w.dir}`,
  title: w.name,          // the object's own name
  group: 'Workspaces',    // its real container
  kind: 'workspace',      // the muted type word at the row's right edge
  scope: 'goto',
  icon: IconFolder,
  run: () => open(w.dir),
}))))
```

**Go-to rows are global.** A row meant to be reachable from anywhere
registers at App level, in `useAppPaletteRows`, off a module-scoped source
(`useTerminalSessions`, `useAttachedTerminalWindows`, `useAgentSessionsAll`),
so it exists on a fresh launch before its mode has ever mounted. A row
registered inside a lazily-mounted mode does not exist until the user visits
that mode.

A row registered inside a mode (`TerminalMode.vue`) is correct only when it
acts on state that lives in that component. Gate it on `active` inside the
getter: a mode is mounted once and then hidden.

Row shape:

- **An object row is titled with the object's own name** and grouped under
  its real container. Never encode a path or a verb in the title:
  `"Desktop"`, not `"Switch to profile: Desktop"`.
- `kind` is for object rows only.
- `scope: 'goto'` browses, `scope: 'actions'` acts. Default is `actions`.
- `order` places the group: lower sorts earlier, and a group sorts as early
  as its earliest row asks.
- **Ids must be stable across renders.** Recents are stored per row id in
  localStorage, so an id built from an index or a timestamp breaks them.
  Dynamic rows use colons (`terminal:session:kill`); catalog ids use dots.
- **Hidden, never disabled.** A row that cannot run where the user is
  standing is filtered out by its getter, not greyed
  (ADR quick-terminal-launchers-are-session-scoped).

Adding a scope tab is one entry in `cmd/desktop/frontend/src/palette/scopes.ts`.
A sigil (`@`, `>`, `!`, `?`) is grammar: `setQuery` absorbs it only as the
first character of an empty query.

## 7. Verify

```bash
mise run desktop:frontend:test
```

Where to add tests:

- `composables/__tests__/useKeybindings.spec.ts`: resolution and conflicts;
- `components/__tests__/KeybindingSettingsView.spec.ts`: the editor row;
- `src/__tests__/App.spec.ts`: dispatch, sequences, and the global Go-to rows;
- `composables/__tests__/useCommands.spec.ts`: scoring and the registry;
- `components/__tests__/CommandPalette.spec.ts`: rendering.

Then check by hand in a running app (`mise run desktop:dev`):

1. a new command appears in Settings ▸ Keyboard, rebinds cleanly, and the key
   does something (the `runMap` pair);
2. a new row appears from a cold start, in a mode you have not visited yet.

## Report

State what you added, and state what you decided **not** to add and why.
"This is a widget-local key", "this earns a catalog entry but no default
chord", and "this is an object row, not a command" are all conclusions worth
writing down, because the next audit re-asks the same questions.
