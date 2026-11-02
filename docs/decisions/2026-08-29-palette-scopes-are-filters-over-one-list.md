# Palette scopes are filters over one list

- **Status:** accepted
- **Date:** 2026-08-29

## Context

The palette already filters its rows to the view on screen (`contextActive`),
which answers "what can I do here" but not "show me only what I can jump to"
from wherever the user stands. The `!` shell escape was a sigil grammar nothing
surfaced.

## Decision

**A scope is a filter predicate over the one registered command list, never a
second list or a page to drill into.** In `useCommandPalette`
(`composables/useCommands.ts`), Go to and Actions add `cmd.scope === X` to the
same filter All uses, so selecting either cannot show a row All would not. A
user who never learns the scopes still reaches everything. A scope is chosen
explicitly and is independent of which view is mounted: Go to lists sessions,
windows, settings sections, and chats the same way in the feed or in Code. The
Shell and Keys scopes are exceptions: each answers from its own source (the
typed command line, the keymap) and shows only where one is registered.

Drive is the other exception. Its document rows sit on the one list, but only
the Drive scope shows them: All and Go to leave them out, because a drive holds
enough documents to bury every command. A document is reached through the tab,
`#`, or `palette.drive`.

The tab strip is the primary surface; sigils (`@`, `>`, `#`, `!`, `?`, declared in
`palette/scopes.ts`) are a fast path into the same scopes. `setQuery` treats a
sigil as an entry only as the first character of an empty query, so a literal
`@` mid-edit stays text. `Tab` / `Shift+Tab` cycle the visible scopes, and
`Backspace` on an empty query returns to All.

**A row meant to be reachable from every scope registers at the App level from
a module-scoped composable, never inside a lazily mounted view.**
`useAppPaletteRows` is that place: the `terminal:attach:*` and
`terminal:window:*` rows read `useTerminalSessions` and
`useAttachedTerminalWindows`, so they exist before the Code view has mounted.

## Consequences

- A new kind of global object gets its palette rows in `useAppPaletteRows`
  against a module singleton, not in the component that first renders it.
- Adding a scope is one entry in `palette/scopes.ts` plus a branch in the
  `results` switch; the tab strip, cycling, and sigil handling are generic.
