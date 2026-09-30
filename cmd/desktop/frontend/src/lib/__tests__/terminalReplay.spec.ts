import { describe, expect, it } from 'vitest'
import { Terminal } from '@xterm/xterm'

// The byte layout a first paint arrives in is decided in Go
// (internal/app/tmuxcc/client.go, ADR terminal-first-paint-carries-scrollback), but whether it lands correctly is
// decided by xterm. These tests pin the emulator behaviour that layout is built
// on, so a change to either side has something to fail against.

const COLS = 20
const ROWS = 5

function replay(term: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => term.write(data, () => resolve()))
}

function freshTerminal(): Terminal {
  return new Terminal({ cols: COLS, rows: ROWS, scrollback: 1000 })
}

function viewport(term: Terminal): string[] {
  const buffer = term.buffer.active
  return Array.from({ length: ROWS }, (_, row) =>
    buffer.getLine(buffer.baseY + row)?.translateToString(true) ?? '')
}

function scrollback(term: Terminal): string[] {
  const buffer = term.buffer.active
  return Array.from({ length: buffer.baseY }, (_, row) =>
    buffer.getLine(row)?.translateToString(true) ?? '')
}

/** The Go snapshot's layout: history rows, exactly ROWS screen rows, private modes, cursor. */
function snapshot(history: string[], screen: string[], cursor: { row: number; col: number }, modes = ''): string {
  return [...history, ...screen].join('\r\n') + modes + `\x1b[${cursor.row + 1};${cursor.col + 1}H`
}

describe('first-paint replay', () => {
  it('scrolls history out of the viewport and leaves the screen filling it', async () => {
    const term = freshTerminal()
    const history = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6']
    const screen = ['s0', 's1', '', '', 's4']

    await replay(term, snapshot(history, screen, { row: 1, col: 2 }))

    expect(viewport(term)).toEqual(screen)
    expect(scrollback(term)).toEqual(history)
    expect([term.buffer.active.cursorY, term.buffer.active.cursorX]).toEqual([1, 2])
  })

  it('restores mouse tracking without giving up the normal-buffer scrollback', async () => {
    const term = freshTerminal()
    const history = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6']
    const screen = ['nvim', '~', '~', '~', '~']

    await replay(term, snapshot(history, screen, { row: 0, col: 4 }, '\x1b[?1002;1006h'))

    expect(term.buffer.active.type).toBe('normal')
    expect(scrollback(term)).toEqual(history)
    expect(term.modes.mouseTrackingMode).toBe('drag')
  })

  // The alignment claim, stated as the thing it protects: a program redrawing
  // from home overwrites the pane's own first row and nothing above it.
  it('lands a redraw from home on the pane row it was aimed at', async () => {
    const term = freshTerminal()
    const history = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6']
    await replay(term, snapshot(history, ['s0', 's1', 's2', 's3', 's4'], { row: 0, col: 0 }))

    await replay(term, '\x1b[H' + 'REDRAWN')

    expect(viewport(term)).toEqual(['REDRAWN', 's1', 's2', 's3', 's4'])
    expect(scrollback(term)).toEqual(history)
  })

  // The negative control, and the bug this replaces: painting the screen short
  // — trimming its blank rows — leaves the viewport straddling history, so the
  // same redraw overwrites scrollback and the pane's rows sit below where the
  // program believes they are, by however many rows were trimmed.
  it('misaligns that redraw by as many rows as a short screen leaves out', async () => {
    const term = freshTerminal()
    const history = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6']

    await replay(term, [...history, 's0', 's1', 's2'].join('\r\n'))
    await replay(term, '\x1b[H' + 'REDRAWN')

    expect(viewport(term)).toEqual(['REDRAWN', 'h6', 's0', 's1', 's2'])
  })
})
