# A terminal pane claims its atlas renderer on activation, not on mount

- **Status:** accepted
- **Date:** 2026-07-30
- **Amends:** [terminal-atlas-renderer](2026-07-29-terminal-atlas-renderer.md) decision 1

## Context

ADR terminal-atlas-renderer put every pane on an atlas renderer and loaded it in `attachTab`, the
moment the pane mounts. ADR terminal-attach-pool then made the terminal view hold several
sessions attached at once, and every pooled session mounts a pane per tmux
window so its screen survives a switch. The two compose badly: the number of
live WebGL contexts is the *total window count across the pool*, not the one
pane on screen. At the shipped defaults — pool of 3, agent sessions carrying
four windows — that is a dozen contexts to display one.

WebKit reports the result on every cold attach:

```
There are too many active WebGL contexts on this page, the oldest context
will be lost.
```

ADR terminal-atlas-renderer decision 3 anticipated exactly this and is why it is not a
correctness bug: the pane whose context is taken claims the canvas renderer,
which shares `TextureAtlas` with the WebGL one, so #131 does not come back.
What decision 3 could not fix is the cost of getting there. Each attach now
takes a context away from a pane in a *different* session, which disposes its
addon and re-rasterises a glyph atlas on the main thread — during the switch
the user is waiting on.

`@xterm/addon-webgl` never calls `WEBGL_lose_context.loseContext()`, so
disposal only drops references and reclamation waits for GC. Evicted sessions'
contexts therefore outlive their eviction, which is why the limit is reached on
every attach rather than occasionally.

## Decision

1. **A pane claims its renderer when its window is first shown, not when its
   pane mounts.** `attachTab` claims one only for the window that is already
   active; `setActive` claims one for any window that has a mounted host and no
   renderer yet. This supersedes ADR terminal-atlas-renderer decision 1's *timing* — that every
   pane loads an atlas renderer — and keeps its substance: no pane is ever
   *displayed* on the DOM renderer. A window nobody opens costs no context.

   Contexts are not released on deactivation. Unloading on the way out would
   put a context teardown and a fresh atlas rasterisation on every tab flip,
   which is the hot path the pool exists to keep instant. The bound moves from
   "every window of every pooled session" to "every window actually visited in
   a pooled session", which is the smaller number in every real layout.

2. **A failed canvas claim is recorded, and the next activation retries.**
   `loadRendererAddon` swallows a throw and returns `undefined`, so before this
   a pane whose post-context-loss canvas claim failed sat on the DOM renderer
   permanently and silently — the one path by which #131 could still reach a
   user. `TabRuntime.rendered` tracks whether an atlas renderer is live, and a
   false value makes the next activation try again.

## Consequences

- **Fewer contexts, and no cross-session eviction on the common path.** One
  visited window per pooled session rather than every window of it, so the
  per-page limit is reached far later, and an attach stops taking a context
  away from a pane in another session.
- **The renderer load moves off the attach and onto the first activation of
  each window.** It was 4–6ms per window measured; a four-window attach stops
  paying three of them. That is small next to the attach itself — the reason
  for this change is the context budget, not the milliseconds.
- **A background window renders through the DOM renderer until it is first
  shown.** It is not on screen, and the atlas renderer repaints the viewport
  from the buffer when it takes over, so nothing is displayed through it. Note
  that a background pane still *renders* — this bounds the GL cost, not the
  work xterm does for hidden panes.
- **`rendered` is the pane's renderer state and has to stay truthful.** Any new
  path that loads or drops a renderer addon must maintain it, or a pane will
  either be skipped by the retry or claim a second context.
