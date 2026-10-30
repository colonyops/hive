# A revealed terminal draws in the tick that reveals it

- **Status:** accepted
- **Date:** 2026-10-30

## Context

Hidden panes stay mounted so a switch is a `v-show` flip (ADR
terminal-attach-pool). xterm stops rendering a hidden terminal and resumes from an
IntersectionObserver callback, which runs after the frame that revealed it, and it
then waits another animation frame to draw. A TUI that redrew while hidden showed
its old screen for two frames on every switch, which reads as a flash.

## Decision

**The window and the pool draw the shown panes in the `nextTick` after the DOM
flip**, which runs before the browser paints (`renderRevealedTerminal`). It does
what xterm's callback does, one frame sooner, through private fields of its render
service. It must clear `_isNextRenderRedrawOnly`, because the public `onRender` is
gated on it and the Kitty image layer listens to it.

Keeping hidden panes rendering would also close the gap, and would spend the GPU on
every pooled pane's output.

## Consequences

- Stale frames went from 2 to 0. A tab's flip takes 3 ms and lands on the next
  frame, 16 ms, in a real session with nvim and top behind the tabs. A switch makes
  neither program redraw.
- An xterm upgrade that moves these fields falls back to the flash and nothing
  worse. `switchWindow` in the perf lane asserts 0 stale frames, so it fails loudly.
