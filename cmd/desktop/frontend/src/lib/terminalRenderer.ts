import { WebglAddon } from '@xterm/addon-webgl'
import type { ITerminalAddon, Terminal } from '@xterm/xterm'
import { createLogger } from './log'

const log = createLogger('terminal-renderer')

/**
 * Put the WebGL atlas renderer on `term`, leaving xterm's DOM renderer as the
 * fallback. The DOM renderer cannot join box drawing or underlines across
 * cells (ADR terminal-atlas-renderer), and xterm 6 removed the canvas renderer
 * that used to sit between the two (ADR
 * xterm-6-drops-the-canvas-renderer-so-the-dom-renderer-is-the-only-fallback).
 *
 * Call this **after** `term.open(host)`, never before: an unopened Terminal
 * makes the addon defer its activation to `open()` via `onWillOpen`, which
 * throws the missing-context error out of `open()` rather than out of
 * `loadAddon` — past the `try` the fallback depends on.
 *
 * `track` takes the addon. It must be disposed with the caller and ahead of
 * the Terminal: xterm disposes its core before its addons, and the addon
 * restores the DOM renderer on the way out. `record` reports whether WebGL is
 * live, on the claim and again after a context loss, so the caller's next
 * reveal can retry a failed claim (ADR terminal-renderer-claimed-on-activation).
 */
export function claimAtlasRenderer(
  term: Terminal,
  track: (addon: ITerminalAddon) => void,
  record: (rendered: boolean) => void,
): void {
  let webgl: WebglAddon
  try {
    webgl = new WebglAddon()
    term.loadAddon(webgl)
  } catch (error) {
    log.warn('WebGL renderer unavailable, using the DOM renderer', error)
    record(false)
    return
  }
  track(webgl)
  record(true)
  // Fires only when the browser did not restore the context on its own; past
  // the browser's context limit the oldest pane's context is the one dropped.
  webgl.onContextLoss(() => {
    log.warn('WebGL context lost, using the DOM renderer')
    webgl.dispose()
    record(false)
  })
}
