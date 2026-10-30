// @xterm/addon-fit measures one pane host. Window votes instead reuse xterm's
// private cell metrics against the full window box.

import type { Terminal } from '@xterm/xterm'

export interface CellSize {
  width: number
  height: number
}

export interface GridSize {
  cols: number
  rows: number
}

interface TerminalCore {
  _renderService?: { dimensions?: { css?: { cell?: { width: number; height: number } } } }
}

function core(term: Terminal): TerminalCore | undefined {
  return (term as unknown as { _core?: TerminalCore })._core
}

/** The rendered cell of an opened terminal, or null before it has measured one. */
export function terminalCellSize(term: Terminal): CellSize | null {
  const cell = core(term)?._renderService?.dimensions?.css?.cell
  if (!cell?.width || !cell.height) return null
  return { width: cell.width, height: cell.height }
}

export function proposeGrid(box: { width: number; height: number }, cell: CellSize): GridSize | null {
  if (!box.width || !box.height || !cell.width || !cell.height) return null
  const cols = Math.floor(box.width / cell.width)
  const rows = Math.floor(box.height / cell.height)
  if (cols < 1 || rows < 1) return null
  return { cols, rows }
}

interface RenderService {
  _isPaused?: boolean
  _needsFullRefresh?: boolean
  _isNextRenderRedrawOnly?: boolean
  _charSizeService?: { hasValidSize?: boolean; measure?: () => void }
  _pausedResizeTask?: { flush?: () => void }
  _renderRows?: (start: number, end: number) => void
  _needsSelectionRefresh?: boolean
  _selectionState?: { start: unknown; end: unknown; columnSelectMode: boolean }
  handleSelectionChanged?: (start: unknown, end: unknown, columnSelectMode: boolean) => void
}

/**
 * Draws a terminal that has just become visible, before the browser paints it.
 *
 * xterm stops rendering a hidden terminal and resumes from an IntersectionObserver
 * callback, which runs after the frame that revealed it. It then waits another
 * animation frame to draw. A pane that took output while hidden therefore shows
 * its old screen for two frames, which reads as a flash on a TUI that redraws.
 * This does what that callback does, one frame sooner. The observer still runs
 * and finds nothing left to do. An xterm whose internals moved falls back to the
 * flash and nothing worse.
 */
export function renderRevealedTerminal(term: Terminal): void {
  if (!term.element?.offsetParent) return
  const service = (term as unknown as { _core?: { _renderService?: RenderService } })._core?._renderService
  if (!service?._isPaused || typeof service._renderRows !== 'function') return
  service._isPaused = false
  // A pane opened while hidden measured nothing, so its renderer has no grid yet.
  if (service._charSizeService && !service._charSizeService.hasValidSize) service._charSizeService.measure?.()
  if (!service._needsFullRefresh) return
  service._needsFullRefresh = false
  service._pausedResizeTask?.flush?.()
  // The public onRender is gated on this, and the image layer and the textarea
  // sync listen to it.
  service._isNextRenderRedrawOnly = false
  service._renderRows(0, term.rows - 1)
}

/**
 * Stops a hidden terminal redrawing its rows for a selection.
 *
 * Every other render path honors xterm's pause, but this one calls the renderer
 * directly. A pane holding a selection refreshes it as output trims its full
 * scrollback, and the DOM renderer a hidden pane sits on measures width 0 inside
 * display:none. Its cache keeps only widths above 0, so it measures every cell
 * again, each time.
 * The state is kept and drawn on reveal. An xterm whose internals moved keeps its
 * own behavior.
 */
export function deferHiddenSelectionRedraw(term: Terminal): void {
  const service = (term as unknown as { _core?: { _renderService?: RenderService } })._core?._renderService
  const original = service?.handleSelectionChanged
  if (!service?._selectionState || !original) return
  service.handleSelectionChanged = (start, end, columnSelectMode) => {
    if (term.element?.offsetParent || !service._selectionState) {
      original.call(service, start, end, columnSelectMode)
      return
    }
    service._selectionState.start = start
    service._selectionState.end = end
    service._selectionState.columnSelectMode = columnSelectMode
    service._needsSelectionRefresh = true
    service._needsFullRefresh = true
  }
}
