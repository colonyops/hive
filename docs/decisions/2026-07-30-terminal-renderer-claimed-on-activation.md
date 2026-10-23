# A terminal pane claims its atlas renderer on activation, not on mount

- **Status:** accepted
- **Date:** 2026-07-30

## Context

ADR terminal-atlas-renderer puts every pane on an atlas renderer. ADR
terminal-attach-pool keeps several sessions attached, each mounting a pane per
tmux window. Claiming a WebGL context on mount makes the live context count the
total pane count across the pool, and WebKit then reports "too many active
WebGL contexts" on every cold attach and takes a context from a pane in a
different session. That pane falls back to the canvas renderer and
re-rasterises a glyph atlas on the main thread, during the switch the user is
waiting on. `@xterm/addon-webgl` never calls `loseContext()`, so evicted
contexts linger until GC.

## Decision

1. **A pane claims its renderer when its window is first shown, not when it
   mounts** (`useTerminalWindows.ts`). `attachPane` claims one only for a pane
   of the active window; `setActive` claims one for every mounted pane of the
   newly active window that has none. No pane is ever displayed on the DOM
   renderer, and a window nobody opens costs no context.

   Contexts are not released on deactivation: unloading would put a teardown
   and a fresh atlas on every tab flip, the hot path the pool keeps instant.

2. **A failed claim is recorded, and the next activation retries.**
   `PaneRuntime.rendered` tracks whether an atlas renderer is live. A failed
   canvas claim after a context loss would otherwise leave the pane on the DOM
   renderer for good, the one path by which #131 could still reach a user.

## Consequences

- The context bound is the panes actually visited per pooled session, and an
  attach stops taking a context from another session on the common path.
- A background window renders through the DOM renderer until first shown. The
  atlas renderer repaints from the buffer when it takes over. This bounds the
  GL cost, not xterm's work for hidden panes.
- `rendered` must stay truthful. Any new path that loads or drops a renderer
  addon must maintain it, or a pane is skipped by the retry or claims a second
  context.
