# xterm 6 drops the canvas renderer, so the DOM renderer is the only fallback

- **Status:** accepted (supersedes decisions 1 and 3 of
  [terminal-atlas-renderer](2026-07-29-terminal-atlas-renderer.md) in part)
- **Date:** 2026-10-15

## Context

The frontend moves to `@xterm/xterm` 6.0.0. xterm 6 removed the canvas
renderer, and `@xterm/addon-canvas` has no release for it. ADR
terminal-atlas-renderer chained WebGL, then canvas, then DOM, and sent a lost
WebGL context to canvas.

## Decision

A pane loads `@xterm/addon-webgl`, and the DOM renderer is the fallback. A lost
WebGL context disposes the addon, which leaves the DOM renderer in place, and
records the pane as unrendered so its next activation claims WebGL again (ADR
terminal-renderer-claimed-on-activation).

## Consequences

- An environment without WebGL2, and a pane past the browser's WebGL context
  limit, draws through the DOM renderer and shows #131's seams again. No atlas
  renderer is left to fall back to.
- The rest of terminal-atlas-renderer stands: load after `open()`, fonts
  resident before open, and WebGL as the renderer of record.
