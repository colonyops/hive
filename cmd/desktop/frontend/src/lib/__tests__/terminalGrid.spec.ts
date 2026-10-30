import { describe, expect, it, vi } from 'vitest'
import type { Terminal } from '@xterm/xterm'
import { deferHiddenSelectionRedraw, proposeGrid, renderRevealedTerminal, terminalCellSize } from '../terminalGrid'

const cell = { width: 10, height: 25 }

function terminalWithCell(size: { width: number; height: number } | undefined): Terminal {
  const core = size === undefined ? {} : { _renderService: { dimensions: { css: { cell: size } } } }
  return { _core: core } as unknown as Terminal
}

describe('proposeGrid', () => {
  it('counts the whole cells a box holds', () => {
    expect(proposeGrid({ width: 1200, height: 1000 }, cell)).toEqual({ cols: 120, rows: 40 })
    expect(proposeGrid({ width: 1209, height: 1024 }, cell)).toEqual({ cols: 120, rows: 40 })
  })

  it('proposes nothing for a box narrower or shorter than one cell', () => {
    expect(proposeGrid({ width: 9, height: 1000 }, cell)).toBeNull()
    expect(proposeGrid({ width: 1200, height: 24 }, cell)).toBeNull()
  })

  it('proposes nothing for a box with no size', () => {
    expect(proposeGrid({ width: 0, height: 0 }, cell)).toBeNull()
    expect(proposeGrid({ width: 0, height: 1000 }, cell)).toBeNull()
  })

  it('proposes nothing before a cell has been measured', () => {
    expect(proposeGrid({ width: 1200, height: 1000 }, { width: 0, height: 0 })).toBeNull()
  })
})

describe('terminalCellSize', () => {
  it('reads the rendered cell', () => {
    expect(terminalCellSize(terminalWithCell({ width: 7.5, height: 17 }))).toEqual({ width: 7.5, height: 17 })
  })

  it('is null before a cell is measured', () => {
    expect(terminalCellSize(terminalWithCell({ width: 0, height: 0 }))).toBeNull()
  })

  it('is null when the private field it reads is missing', () => {
    expect(terminalCellSize(terminalWithCell(undefined))).toBeNull()
    expect(terminalCellSize({} as unknown as Terminal)).toBeNull()
  })
})

function terminal(service: Record<string, unknown>, shown = true): Terminal {
  return {
    rows: 24,
    element: { offsetParent: shown ? {} : null },
    _core: { _renderService: service },
  } as unknown as Terminal
}

describe('renderRevealedTerminal', () => {
  it('draws a paused pane that took output while hidden, and marks the render as public', () => {
    const renderRows = vi.fn()
    const service = { _isPaused: true, _needsFullRefresh: true, _isNextRenderRedrawOnly: true, _renderRows: renderRows }
    renderRevealedTerminal(terminal(service))
    expect(renderRows).toHaveBeenCalledWith(0, 23)
    expect(service._isPaused).toBe(false)
    expect(service._needsFullRefresh).toBe(false)
    expect(service._isNextRenderRedrawOnly).toBe(false)
  })

  it('measures a cell for a pane that opened hidden and has none yet', () => {
    const measure = vi.fn()
    renderRevealedTerminal(
      terminal({
        _isPaused: true,
        _needsFullRefresh: false,
        _charSizeService: { hasValidSize: false, measure },
        _renderRows: vi.fn(),
      }),
    )
    expect(measure).toHaveBeenCalledOnce()
  })

  it('applies a resize that was deferred while hidden before drawing', () => {
    const order: string[] = []
    renderRevealedTerminal(
      terminal({
        _isPaused: true,
        _needsFullRefresh: true,
        _pausedResizeTask: { flush: () => order.push('resize') },
        _renderRows: () => order.push('render'),
      }),
    )
    expect(order).toEqual(['resize', 'render'])
  })

  it('unpauses without drawing when nothing changed while hidden', () => {
    const renderRows = vi.fn()
    const service = { _isPaused: true, _needsFullRefresh: false, _renderRows: renderRows }
    renderRevealedTerminal(terminal(service))
    expect(renderRows).not.toHaveBeenCalled()
    expect(service._isPaused).toBe(false)
  })

  it('leaves a pane that is still hidden paused, because nothing would unpause it again', () => {
    const service = { _isPaused: true, _needsFullRefresh: true, _renderRows: vi.fn() }
    renderRevealedTerminal(terminal(service, false))
    expect(service._isPaused).toBe(true)
    expect(service._renderRows).not.toHaveBeenCalled()
  })

  it('does nothing when xterm internals have moved', () => {
    expect(() => renderRevealedTerminal(terminal({}))).not.toThrow()
    expect(() =>
      renderRevealedTerminal({ rows: 24, element: { offsetParent: {} } } as unknown as Terminal),
    ).not.toThrow()
  })
})

describe('deferHiddenSelectionRedraw', () => {
  function withSelectionService(shown: boolean) {
    const redraw = vi.fn()
    const service = {
      _selectionState: { start: undefined, end: undefined, columnSelectMode: false },
      _needsSelectionRefresh: false,
      _needsFullRefresh: false,
      handleSelectionChanged: redraw,
    } as Record<string, unknown>
    const term = terminal(service, shown)
    deferHiddenSelectionRedraw(term)
    return { service, redraw, term }
  }

  it('keeps the selection for the reveal instead of drawing a hidden pane', () => {
    const { service, redraw } = withSelectionService(false)
    ;(service.handleSelectionChanged as (s: unknown, e: unknown, c: boolean) => void)([0, 1], [5, 1], true)
    expect(redraw).not.toHaveBeenCalled()
    expect(service._selectionState).toEqual({ start: [0, 1], end: [5, 1], columnSelectMode: true })
    expect(service._needsSelectionRefresh).toBe(true)
    expect(service._needsFullRefresh).toBe(true)
  })

  it('draws at once on a pane that is shown', () => {
    const { service, redraw } = withSelectionService(true)
    ;(service.handleSelectionChanged as (s: unknown, e: unknown, c: boolean) => void)([0, 1], [5, 1], false)
    expect(redraw).toHaveBeenCalledWith([0, 1], [5, 1], false)
  })

  it('does nothing when xterm internals have moved', () => {
    expect(() => deferHiddenSelectionRedraw(terminal({}))).not.toThrow()
  })
})
