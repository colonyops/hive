---
name: palette-audit
description: Check the work on this branch (or a PR) for anything that should be reachable from the command palette and is not, then add the rows. Use when asked to audit or tune the palette, to check what ⌘K is missing, or as a pre-PR pass over a feature that added a view, an object, or a verb.
---

# Audit the command palette

⌘K is the app's index. Anything a user can reach by clicking should also be
reachable by typing, and the failure mode is silent: a feature ships, nobody
registers a row, and the palette quietly stops being complete.

The doctrine lives in `docs/architecture.md` ("The command palette's rules sit
beside the keymap doctrine") and three ADRs -- read them before deviating, not
before every audit:
[palette-scopes-are-filters-over-one-list](../../../docs/decisions/2026-08-29-palette-scopes-are-filters-over-one-list.md),
[palette-rows-name-objects-and-rendering-draws-the-path](../../../docs/decisions/2026-08-29-palette-rows-name-objects-and-rendering-draws-the-path.md),
[a-command-is-a-source](../../../docs/decisions/2026-08-05-a-command-is-a-source.md).

## 1. Scope the change

```bash
git diff main...HEAD --stat
git diff main...HEAD -- cmd/desktop/frontend/src
```

For a PR that is not checked out, `gh pr diff <number>`.

## 2. Inventory what a user gained

Read the diff for these, and write them down before deciding anything:

- **verbs** -- a new button, menu entry, context-menu item, toolbar icon, or
  anything with an `@click` a user can reach;
- **objects** -- a new kind of thing that can be listed and selected (a
  workspace, a canvas, a pinned chat, a window, a source);
- **places** -- a new view, overlay, dialog, mode, or settings section;
- **configured things** -- a new action type, launcher, or flow surface that a
  user's `actions.yml` can now name.

An internal refactor, a change to how an existing row is drawn, and a
backend-only change all inventory to nothing. Say so and stop -- an audit that
finds nothing is a valid outcome and should be reported as one.

## 3. Classify each one

**A verb the user could want to rebind is a command.** It belongs in
`cmd/desktop/frontend/src/keybindings/catalog.ts`, and `useAppPaletteRows` seeds
its palette row from there automatically -- title, group, keywords, icon and
live shortcut hint, all from the one declaration. Adding it is the
`keymap-audit` skill's procedure; here your job is to confirm the row actually
appears: it is not `paletteHidden`, and its `context` is active wherever the
verb makes sense.

**A verb that only exists where its object does is a row, not a command.**
Killing the attached session, unpinning this chat, deleting this workspace:
these act on state a component owns and mean nothing without it, so they
register as `scope: 'actions'` rows from that component and are not bindable.
`TerminalMode.vue`'s `terminal:session:kill` is the pattern. Group them under
the object's own name so the palette reads as that object's operations.

**An object is data, not a command.** Feeds, profiles, sessions, windows,
chats, workspaces, themes, settings sections: register these with `useCommands`
as a reactive source. They are not bindable and must not be added to the
catalog.

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

**A configured action is neither.** Actions from `actions.yml` already reach
the palette through the selected item's own rows, and launchers already
contribute `launcher.<id>` commands through `setLauncherCommands`. A new
action *type* needs nothing; check it flows through those paths rather than
building a parallel one.

## 4. Register it in the right place

**Go-to rows are global.** A row meant to be reachable from anywhere registers
at App level -- `useAppPaletteRows`, off a module-scoped source
(`useTerminalSessions`, `useAttachedTerminalWindows`, `useAgentSessionsAll`) --
so it exists on a fresh launch before its mode has ever mounted. A row
registered inside a lazily-mounted mode component does not exist until the user
has visited that mode, which is exactly the gap the palette is meant to close.

A row registered inside a mode (`TerminalMode.vue`) is correct only when it
acts on state that lives in that component. Gate it on `active` inside the
getter rather than relying on scope disposal: a mode is mounted once and then
hidden.

## 5. Get the row's shape right

- **A command row keeps its catalog verb title unchanged.** Settings ▸ Keyboard
  reads the same string, so the two surfaces cannot be allowed to drift.
- **An object row is titled with the object's own name** and grouped under its
  real container. Rendering draws the nesting -- the group header, or a
  `Container ›` prefix once a query narrows past it. Never encode a path or a
  verb in the title: `"Desktop"`, not `"Switch to profile: Desktop"`.
- `kind` is the muted type word Enter lands on (`window`, `feed`, `chat`).
  Object rows only -- a verb row's title already says what it does.
- `scope: 'goto'` browses, `scope: 'actions'` acts. Default is `actions`.
- `order` places the group: lower sorts earlier, and a group sorts as early as
  its earliest row asks, so registrars in different files need not agree.
- **Ids must be stable across renders.** Recents are stored per row id in
  localStorage, so an id built from an array index or a timestamp silently
  breaks them. Dynamic rows use colons (`terminal:session:kill`); catalog
  command ids use dots (`terminal.close-window`).

## 6. Hidden, never disabled

A row that cannot run where the user is standing is filtered out of the list --
not shown greyed. Gate the getter, do not add a `disabled` flag. This is the
choice the launcher rows already made
(ADR quick-terminal-launchers-are-session-scoped).

## 7. Scopes

Adding a scope tab is one entry in `cmd/desktop/frontend/src/palette/scopes.ts` and
nothing else -- not a change scattered across `useCommands` and
`CommandPalette.vue`. A sigil (`@`, `>`, `!`, `?`) is grammar, not a command:
`setQuery` absorbs it only as the first character of an empty query. `Shell`
and `Keys` claim their tabs through `useShellEscape` / `useKeysScope`, which
also decide whether the tab is visible.

## 8. Verify

Add or extend a test with the row -- `src/__tests__/App.spec.ts` covers the
global Go-to rows, `composables/__tests__/useCommands.spec.ts` the scoring and
registry, `components/__tests__/CommandPalette.spec.ts` the rendering.

```bash
cd cmd/desktop/frontend && npm test
```

Then open the palette in a running app and check the row from a cold start, in
a mode you have not visited yet -- that is the case the global-registration
rule exists for, and the one a unit test is least likely to catch.

## Report

State what you added, and state explicitly what you decided **not** to add and
why. A palette audit that lists every rejected candidate is more useful than
one that only lists additions, because the next audit re-asks the same
questions.
