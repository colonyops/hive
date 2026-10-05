# Agents outside every session share one global canvas owner

- **Status:** accepted
- **Date:** 2026-10-04

## Context

A canvas belongs to a Chats workspace or to the repository of a hive session
(ADR a-code-session-s-canvases-belong-to-its-repository-and-live-in-the-hive-context-directory).
Hive also runs agents that sit in neither: an agent started in a plain
terminal, or one the user keeps for work across repositories. Such an agent
had no owner to write under, so every canvas tool answered `not_found`.

## Decision

**An agent outside every session passes `session: "global"`.** All such
agents share one owner, `@global`. The `@` keeps the key out of the names
hive gives a workspace directory, and the missing slash keeps it apart from a
repository key. The canvases are filed in the desktop's data directory, at
`<data dir>/canvases/<name>.json`. They belong to the app, not to a workspace
folder or a repository's context directory.

**A working directory inside no hive session stays `not_found`.** It does not
fall back to the global owner. Until the hive engine starts, no directory is
inside a session. A fallback would then file a session agent's canvases where
its pane never looks. The error names `global`, so an agent that is truly
outside a session corrects itself in one call.

**A global canvas has no pane.** There is no chat or session for a pane to
sit beside, so `open_canvas` and `close_canvas` refuse it. The full-page view
and the command palette list the global owner beside the workspaces and the
repositories.

## Consequences

A global write records no author, so the canvas events carry neither a chat
nor a hive session. A pane ignores them, and the full-page view still wakes
on them. Two global agents that pick the same canvas name write to the same
canvas. That is the shared namespace the decision asks for.
