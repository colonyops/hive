# xterm 6 drops the canvas renderer, so the DOM renderer is the only fallback

- **Status:** accepted
- **Date:** 2026-10-15

## Context

The frontend runs `@xterm/xterm` 6, which removed the canvas renderer, and
`@xterm/addon-canvas` has no release for it. ADR terminal-atlas-renderer
chained WebGL, then canvas, then DOM.

## Decision

A pane loads `@xterm/addon-webgl`, and the DOM renderer is the fallback. A lost
WebGL context disposes the addon, which leaves the DOM renderer in place, and
records the pane as unrendered so its next activation claims WebGL again (ADR
terminal-renderer-claimed-on-activation).

## Consequences

An environment without WebGL2, and a pane past the browser's WebGL context
limit, draws through the DOM renderer and shows its box-drawing and underline
seams. The rest of ADR terminal-atlas-renderer stands: load after `open()`,
fonts resident before open, and WebGL as the renderer of record.
