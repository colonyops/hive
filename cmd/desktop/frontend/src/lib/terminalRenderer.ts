import { CanvasAddon } from '@xterm/addon-canvas'
import { WebglAddon } from '@xterm/addon-webgl'
import type { ITerminalAddon, Terminal } from '@xterm/xterm'

/**
 * Put an atlas renderer on `term` — WebGL, falling back to canvas, with xterm's
 * DOM renderer only as the last resort.
 *
 * The DOM renderer paints box drawing from the font's own glyphs and underlines
 * as text-decoration on per-cell inline-block spans, so neither joins across
 * cells at any font size or device pixel ratio. An atlas renderer strokes both
 * to the cell's own device-pixel bounds. ADR terminal-atlas-renderer.
 *
 * Call this **after** `term.open(host)`, never before: an unopened Terminal
 * makes the addon defer its activation to `open()` via `onWillOpen`, which
 * throws the missing-context error out of `open()` rather than out of
 * `loadAddon` — past the `try` the fallback depends on.
 *
 * `track` takes every addon loaded. They must be disposed with the caller and
 * ahead of the Terminal: xterm disposes its core before its addons, and these
 * restore a renderer on the way out. `record` reports whether one is live, on
 * the claim and again after a context loss — a failed claim is recorded rather
 * than swallowed so the caller's next reveal can retry it, which is the
 * invariant ADR terminal-renderer-claimed-on-activation asks of every path that claims a renderer.
 */
export function claimAtlasRenderer(
  term: Terminal,
  track: (addon: ITerminalAddon) => void,
  record: (rendered: boolean) => void,
): void {
  const webgl = loadAddon(term, track, () => new WebglAddon())
  if (!webgl) {
    record(claimCanvas(term, track))
    return
  }
  record(true)
  // Fires only when the browser did not restore the context on its own. The
  // addon puts the DOM renderer back as it goes, so claim the canvas instead.
  // This is also the path a session with many windows takes: one WebGL context
  // per open tab, and past the browser's limit the oldest is dropped.
  webgl.onContextLoss(() => {
    webgl.dispose()
    record(claimCanvas(term, track))
  })
}

function claimCanvas(term: Terminal, track: (addon: ITerminalAddon) => void): boolean {
  return loadAddon(term, track, () => new CanvasAddon()) !== undefined
}

function loadAddon<T extends ITerminalAddon>(
  term: Terminal,
  track: (addon: ITerminalAddon) => void,
  create: () => T,
): T | undefined {
  try {
    const addon = create()
    term.loadAddon(addon)
    track(addon)
    return addon
  } catch (error) {
    console.warn('Terminal renderer unavailable, falling back', error)
    return undefined
  }
}
