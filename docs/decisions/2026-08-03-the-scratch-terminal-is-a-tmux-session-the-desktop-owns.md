# The scratch terminal is a tmux session the desktop owns, pinned above the repositories

- **Status:** accepted
- **Date:** 2026-08-03

## Context

Every tmux terminal in the app belongs to a hive session, and a pop-up
terminal dies with the app (ADR ephemeral-popup-terminals). There was no
shell that outlives the app, keeps tabs, and is tied to no tracked work. A hive
session cannot exist without a repository, and its working directory comes from
spawn rules keyed by a remote a scratch shell would not have.

## Decision

1. **The scratch terminal is a tmux session the desktop creates, with no hive
   session behind it.** A remote-less hive session would put a row in `hive.db`
   that the CLI lists as work. As an ordinary tmux session, attach, the window
   sweep, tabs, the pool, and the tree serve it unchanged. The desktop owns only
   its creation (`TerminalsService.startScratch`).
2. **Its slug is `Scratch` (`ScratchSlug`), which hive cannot mint.** Hive's
   `Slugify` lowercases, so no hive session reaches a capital letter. Both
   products share tmux's session namespace.
3. **It starts in the user's home directory as the session's working
   directory**, so every tab added later starts there too. No spawn rule applies.
4. **It runs an interactive login shell**, so the user's startup files decide
   its PATH. The resolved environment tmux is spawned in
   (ADR tmux-runs-in-the-resolved-environment) is only the floor under them.
5. **It is a pinned section at the top of the session tree.** It is a group of
   one session, prepended, so the pool, sweep, and keyboard walk carry it
   without a second kind of row. The section heading (`Terminals`) stands in for
   the session row, and the tabs sit directly under it.
6. **`+` on the heading creates the session when tmux has none**, and that
   session is the first tab. This is scratch-only: `+` on a hive session never
   starts it, because starting runs its agent
   (ADR terminal-start-is-an-offered-action). An empty section still draws one
   row that offers the start.
7. **Liveness comes from the window sweep**, and the row offers only terminal
   lifecycle. Rename, recycle, delete, session details, and configured actions
   address a hive record and are not offered.

## Consequences

- Prune and the prunable count read hive's listing, so the scratch terminal is
  never counted.
- The pool may evict its attach. The tmux session survives; it costs a
  re-attach.
- Killing it is offered and confirmed. Nothing in its tabs persists beyond
  what tmux holds.
- There is exactly one. Tabs are the multiplicity.
- The reserved slug is a contract across two products. A test pins that hive's
  slugify cannot produce it.
