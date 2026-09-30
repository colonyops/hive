# Palette scopes are filters over one list

- **Status:** accepted
- **Date:** 2026-08-29

## Context

`!<cmd>` (ADR quick-terminal-launchers-are-session-scoped's palette) already
carried one sigil into its own mode, but nothing surfaced that a sigil grammar
existed -- a user had to already know `!` opened a shell escape, and had no way
to see or reach the same idea for "jump somewhere" versus "run something".
PR #306 scoped the palette's contents to the view you stood in (feed commands
gone in Code, terminal rows gone on the feed), which answers "what can I do
here" but not "show me only the things I can jump to" from wherever you are.

## Decision

**A scope is a filter predicate over one flat command list, never a second
list or a page to drill into.** `results` stays the single ranked/filtered
view `useCommands` has always produced; a scope only adds `cmd.scope === X` to
its predicate. Selecting a tab cannot show a row that bare `⌘K` -- the "All"
scope -- would not also show. This is the property that keeps the palette safe
to forget: a user who never learns the scopes still reaches everything, and one
who does gets a narrower list of the same rows, not a different app.

This is deliberately not PR #306's mechanism. #306 filters by *where you are*
(`contextActive`) -- implicit, tied to the mounted view, and different lists
per mode. A scope is filtered by *what you asked for* -- explicit, chosen from
the tab strip or a sigil, and orthogonal to mode: Go to lists sessions,
windows, settings sections and chats the same way whether the feed or Code is
on screen. Nothing about a scope switch pushes state the way a drill-in menu
would -- there is no "back", because there was never a "forward".

The tab strip is the primary surface; sigils (`@`, `>`, `!`) are a fast path
into the same scopes, not a separate feature. `useCommandPalette.setQuery`
intercepts a sigil only as the first character of an empty query -- a paste of
`@foo` or a bare `@` enters the scope and keeps typing, but a sigil mid-edit is
just a character, so a user who wants a literal `@` in a search never fights
the grammar. `Tab`/`Shift+Tab` cycle the visible scopes and `Backspace` on an
empty query pops back to All, so a tab reached by accident (sigil or click) is
one keystroke from undone.

Because completeness routes through "All must show it", **a row meant to be
reachable from every scope registers at the App level, sourced from a
module-scoped composable, never from inside a lazily-mounted mode component.**
`useAppPaletteRows` is that registration point: session attach rows and window
rows read `useTerminalSessions`/`useAttachedTerminalWindows` rather than
`TerminalMode`'s own state, precisely so they exist before `TerminalMode` has
ever mounted. A Go-to row that only appeared once its owning view happened to
be on screen would violate the "All reaches everything" guarantee the moment
someone opened the palette from somewhere else.

## Consequences

A new class of global object (another kind of session, another settings
surface) gets its palette rows written into `useAppPaletteRows` against a
module singleton, not into the component that first renders it -- the same
shape `terminal:attach:*` and `terminal:window:*` already follow.

Adding a scope -- the `?` Keys scope was added this way -- is one entry in
`palette/scopes.ts` plus a
branch in `useCommandPalette`'s `results` switch; the tab strip, cycling, and
sigil interception are generic over the registry and need no changes.
