# Agents outside every session share one global canvas owner

- **Status:** accepted
- **Date:** 2026-10-04

## Context

A canvas belongs to a Chats workspace or to the repository of a hive session
(ADR a-code-session-s-canvases-belong-to-its-repository-and-live-in-the-hive-context-directory).
Hive also runs agents that sit in neither: one started in a plain terminal,
or one the user keeps for work across repositories.

## Decision

**An agent outside every session passes `session: "global"`.** All such
agents share the drive's shared space, `<context root>/shared/`
(ADR hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).
Canvas tool results name its owner `@global`, which agents and links already
match on.

**A working directory inside no hive session stays `not_found`.** It does not
fall back to the shared space. Until the hive engine starts, no directory is
inside a session, so a fallback would file a session agent's canvases where
its pane never looks. The error names `global`, so an agent that is truly
outside a session corrects itself in one call.

**A shared canvas has no pane.** No chat or session exists for a pane to sit
beside, so `open_canvas` and `close_canvas` refuse it. The user reads it in
the Drive page.

## Consequences

A shared write records no author, so its events carry neither a chat nor a
hive session, and a pane ignores them. Two agents that pick the same canvas
name write to the same canvas.
