# A pop-up terminal launcher is its own list in actions.yml, not an action

- **Status:** accepted
- **Date:** 2026-07-31

## Context

The pop-up terminal (ADR ephemeral-popup-terminals) launches a directory and a
shell command line. A launcher (a named `lazygit` button, a per-repo command)
is that spec with config in front of it. The action catalog was the obvious
home, as a new `type` or a new `targets` entry.

But every action ends in the `Dispatcher`: an executor, a job with an exit
status and captured streams. A pop-up is a PTY handed to the user. A launcher
type was built first and needed carve-outs across `HasTarget`,
`HeadlessCapable`, `validateActions`, the executor-map bijection and the action
editor, while using none of the action envelope (`targets`, `applies_to`,
`show_in_detail`, `inputs`).

## Decision

1. **A launcher is not an action.** It lives in a sibling `launchers:` list in
   the same `actions.yml`, so one `ActionStore` keeps the single loader,
   watcher, last-good reload, migrations and editor. A `Launcher`
   (`cmd/desktop/internal/app/actions/launcher.go`) carries only `id`, `label`,
   `command`, `cwd` and `icon`. Launcher ids and action ids are separate
   namespaces.

2. **`command` and `cwd` are not templates.** There is no triggering item to
   render over. The command runs through a login shell, so PATH, aliases and
   `$PWD` already resolve it. A leading `~` in `cwd` is expanded. A launcher
   with no `cwd` runs where the terminal on screen is, and is refused with no
   terminal (ADR quick-terminal-launchers-are-session-scoped).

3. **Each launcher is a bindable command, `launcher.<id>`, unbound by
   default.** The frontend catalog merges configured launchers after the
   built-ins, so a combo shared with a built-in resolves to the built-in, and a
   config file cannot take `mod+k` from the palette. Launcher ids are unknown
   until `actions.yml` is read, so the keybinding loader keeps unknown ids
   rather than dropping them (`useKeybindings.ts`); dropping them would erase a
   user's launcher bindings on their next unrelated rebind.

4. **A launcher is opened by id, and the core resolves what it runs.** The
   frontend sends `launcher: "lazygit"` to `/api/terminal/popup/open`, and the
   command and pinned directory come from the catalog at that moment. A
   frontend with a stale catalog cannot run a command the user has since
   changed, and an agent on the loopback API can open a launcher by name.

5. **Invoking a launcher replaces what the pop-up holds.** A launch that
   differs from the live one opens in its place and closes it. Invoking what is
   already on screen hides it, so the chord that opens a launcher also puts it
   away. A launcher's chord fires while a terminal has focus, like the pop-up's
   own toggle.

## Consequences

- `HasTarget`, `HeadlessCapable`, `validateActions` and the executor-map test
  describe actions only. Adding a launcher field touches `Launcher` alone.
- Settings has a separate Launchers section. One read answers both lists, with
  one wake on `actions:updated` and one parse error.
- A flow `action` node cannot name a launcher, since it resolves against the
  action catalog.
- Deleting a launcher needs no usage preflight; a binding for a gone id stops
  resolving.
- Replacing a live pop-up can end a shell someone was using.
- The launcher icon set (`icons.ValidLauncher`, mirrored in
  `frontend/src/lib/launcherIcons.ts`) is kept in sync by hand.
