---
name: keymap-audit
description: Check the work on this branch (or a PR) for anything that should be bindable to a key and is not, then add it to the command catalog and wire it up. Use when asked to audit keybindings or shortcuts, to decide whether a new feature deserves a chord, or as a pre-PR pass over a feature that added a verb or a view.
---

# Audit the keymap

A bindable command is a contract, not a convenience: its id is what a user
writes in `settings.yaml`'s `keybindings:` section, so it outlives every rename
of the code behind it. Adding one is cheap; adding one with a bad id, a stolen
chord, or a default nobody asked for is not.

The doctrine is in `docs/architecture.md` (the keymap section, ending at "The
command palette's rules sit beside the keymap doctrine above") plus ADR
keybindings-are-chord-sequences-not-a-leader-key
([link](../../../docs/decisions/2026-08-29-keybindings-are-chord-sequences-not-a-leader-key.md)).
Read them before deviating, not before every audit.

## 1. Scope the change

```bash
git diff main...HEAD --stat
git diff main...HEAD -- cmd/desktop/frontend/src
```

For a PR that is not checked out, `gh pr diff <number>`.

## 2. Decide what is even a candidate

A candidate is a **stable app verb**: something a user does repeatedly, that
means the same thing every time, and that has one implementation. New views,
overlays, toggles, and lifecycle operations qualify.

These are **not** candidates, and saying so is half the audit:

- **Objects.** Switching to a profile, opening a feed, jumping to a chat.
  Those are palette rows -- see the `palette-audit` skill.
- **Widget-local navigation.** The session tree's arrows and `j`/`k`, the
  palette's own arrows. A combo resolves to exactly one command -- the first in
  the catalog claiming it -- so `context` narrows *when* a command fires but
  never lets two commands share a chord. The tree's `j`/`k` would have had to
  fight the feed's for the same combo, so they stay handlers on the widget that
  owns focus.
- **Anything reached once per session** from a settings page or a dialog.
- **A launcher.** Each `launchers:` entry in `actions.yml` already contributes
  `launcher.<id>` through `setLauncherCommands`, unbound by default. Nothing to
  add.
- **A numbered variant.** `terminal.select-window-1` … `-9` are generated, not
  written out. A tenth is deliberately unreachable.

## 3. Add the catalog entry

`cmd/desktop/frontend/src/keybindings/catalog.ts`, in `commandCatalog`:

The shape, with every field that decides something:

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

`tasks.toggle` in the same file is the worked version of that shape, with a
default chord, a sequence, `piercesPane`, and a comment saying why.

- **`id` is config.** `<area>.<verb>`, lower-case, dots. It is written into a
  user's `settings.yaml` and possibly into a dotfiles repo, so choose it once.
  Do not rename one to match a refactor.
- **`context` gates where a bare, modifier-less binding fires.** `global`,
  `feed`, `terminal`, `terminal-session`, `agents`. It is enforced by
  `contextActive` in `App.vue`; a new context means teaching that switch what
  "active" means for it.
- **Combos are canonical strings, not display glyphs.** `mod+shift+t`,
  `alt+t`, `g t` -- lower-case, modifiers ordered by `canonicalizeCombo`,
  sequence steps space-joined. `mod` is Command on macOS and Ctrl everywhere
  else, which is what makes a `mod` chord worth checking against readline on
  Linux. `formatCombo` turns these into the ⌘-and-⇧ form the UI shows; never
  write that form into the catalog.
- **`defaultCombos: []` is the right default.** A command that is bindable but
  unbound costs a user nothing and takes no chord away. Ship a default only
  when the chord is either a platform standard (`⌘,`, `⌘N`, `⌘[`/`⌘]`) or the
  command is used often enough to earn it. A destructive command with no undo
  stays unbound on purpose -- `feed.mark-workspace-read` is the worked example.
- **Check the combo is free before you claim it.** `commandCatalog` order
  decides ties, so an added entry can silently shadow a later one. Grep the
  file for the combo, and remember launchers are appended after the catalog
  precisely so a config file cannot take `mod+k` from the palette.
- Add `scope: 'goto'` when the command navigates rather than acts, and
  `paletteHidden: true` only when a named dynamic row already stands in for it
  (as `view.focus-search` does per surface).

## 4. Wire the implementation

One entry in `App.vue`'s `runMap`, keyed by the id:

```ts
'tasks.toggle': openTasks,
```

Both the keydown dispatcher and the palette run through that map, so a command
has exactly one implementation and the palette can show its live shortcut.
Launchers and the numbered window jumps are the two exceptions -- each is one
implementation parameterised by what its id names, resolved ahead of the map in
`runCommand`.

**A catalog entry with no `runMap` entry fails silently.** Nothing gates this:
the row appears in the palette, the shortcut appears in Settings, and the key
does nothing. Check the pair by hand.

## 5. Decide about a focused terminal pane

A pane keeps every key it can use. Two flags take one back, and picking the
wrong one is the most common mistake here.

- **`escapesPane`** -- claimed through `terminalEscapeCombo`: Command chords,
  and Ctrl+Shift where there is no Command. **Prefer this.** It is what leaves
  bare Ctrl+K as readline's kill-to-end-of-line while ⌘K is the palette's.
- **`piercesPane`** -- claimed on the binding alone, whatever modifiers it
  carries. Only for a chord the escape form cannot express. An alt binding is
  the case that forces it, since `terminalEscapeCombo` qualifies only Command
  and Ctrl+Shift and a user who binds `alt+t` to an escaping command gets
  nothing. It is also right for a combo that must *close* what it opened
  (`terminal.popup.toggle`, `tasks.toggle`) and for the half of a focus pair
  that reaches back out of a pane.

Both are two-sided: the dispatcher must act on the chord **and** xterm's
`attachCustomKeyEventHandler` must decline it, or the pane writes the key to
tmux as well. Both sides resolve through the live keymap, so a rebind moves
them together. If you cannot answer "why may a pane not have this key?", set
neither flag.

## 6. Sequences

A binding is one combo or a space-separated sequence (`g i`). Navigation to a
place is the established shape -- `g i` / `g c` / `g a` / `g t` / `g s`. A
sequence start answers to the same rule as a bare chord: it never fires over a
focused pane, into an editable target, or under an overlay, and `resolve` stays
single-step, so neither pierce nor escape can claim a sequence's first step out
from under it. Adding a `g <x>` means checking nothing else binds bare `x` in a
context that overlaps.

## 7. Verify

```bash
cd cmd/desktop/frontend && npm test
```

Add to `composables/__tests__/useKeybindings.spec.ts` for resolution and
conflicts, `components/__tests__/KeybindingSettingsView.spec.ts` for the editor
row, `src/__tests__/App.spec.ts` for dispatch and sequences.

Then check by hand, because the two things most likely to be wrong are the two
a test is least likely to catch:

1. the command appears in Settings ▸ Keyboard and rebinds cleanly (the editor
   reports collisions through `kb.conflicts`);
2. the key actually does something -- the `runMap` pair from step 4.

## Report

Say what you added, and say what you deliberately left unbound and why. "This
is a widget-local key" and "this earns a catalog entry but no default chord"
are both conclusions worth writing down, because the next audit will re-ask the
same question.
