import { describe, expect, it } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { resizeTerminalPreservingViewport } from '../terminalViewport'

function write(term: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => term.write(data, resolve))
}

function findLine(term: Terminal, text: string): number {
  const buffer = term.buffer.active
  for (let line = 0; line < buffer.length; line++) {
    if (buffer.getLine(line)?.translateToString(true) === text) return line
  }
  throw new Error(`line not found: ${text}`)
}

function findLineStartingWith(term: Terminal, text: string): number {
  const buffer = term.buffer.active
  for (let line = 0; line < buffer.length; line++) {
    if (buffer.getLine(line)?.translateToString(true).startsWith(text)) return line
  }
  throw new Error(`line not found: ${text}`)
}

function topLine(term: Terminal): string {
  const buffer = term.buffer.active
  return buffer.getLine(buffer.viewportY)?.translateToString(true) ?? ''
}

async function terminalWithWrappedHistory(): Promise<Terminal> {
  const term = new Terminal({ cols: 12, rows: 4, scrollback: 100 })
  term.open(document.createElement('div'))
  await write(term, [
    'before-00000',
    'before-11111',
    'AAAAAAAABBBBCCCCDDDDEEEE',
    'after--00000',
    'after--11111',
    'after--22222',
    'after--33333',
    'after--44444',
  ].join('\r\n'))
  return term
}

describe('resizeTerminalPreservingViewport', () => {
  it('keeps the same wrapped content at the top when narrowing history', async () => {
    const term = await terminalWithWrappedHistory()
    term.scrollToLine(findLine(term, 'CCCCDDDDEEEE'))

    resizeTerminalPreservingViewport(term, 8, 4)

    expect(topLine(term)).toBe('BBBBCCCC')
  })

  it('keeps the same wrapped content at the top when widening history', async () => {
    const term = await terminalWithWrappedHistory()
    term.scrollToLine(findLine(term, 'CCCCDDDDEEEE'))

    resizeTerminalPreservingViewport(term, 20, 4)

    expect(topLine(term)).toBe('AAAAAAAABBBBCCCCDDDD')
  })

  it('accounts for the empty cell before a wrapped double-width glyph', async () => {
    const term = new Terminal({ cols: 5, rows: 4, scrollback: 100 })
    term.open(document.createElement('div'))
    const wideLine = 'abcd界abcd界abcd界abcd界WXYZ界'
    await write(term, [wideLine, 'after-0', 'after-1', 'after-2', 'after-3', 'after-4'].join('\r\n'))
    term.scrollToLine(findLine(term, 'WXYZ'))

    resizeTerminalPreservingViewport(term, 25, 4)

    expect(topLine(term)).toMatch(/W$/)
    expect(term.buffer.active.getLine(term.buffer.active.viewportY + 1)?.translateToString(true)).toBe('XYZ界')
  })

  it('recovers the anchor when reflow trims the start of its logical line', async () => {
    const term = new Terminal({ cols: 10, rows: 4, scrollback: 30 })
    term.open(document.createElement('div'))
    const longLine = `${'a'.repeat(150)}TARGET${'b'.repeat(44)}`
    await write(term, [longLine, 'after-0000', 'after-1111', 'after-2222', 'after-3333', 'after-4444'].join('\r\n'))
    term.scrollToLine(findLineStartingWith(term, 'TARGET'))

    resizeTerminalPreservingViewport(term, 5, 4)

    expect(topLine(term)).toBe('TARGE')
  })

  it('uses the oldest surviving row when reflow trims both boundary markers', async () => {
    const term = new Terminal({ cols: 10, rows: 4, scrollback: 10 })
    term.open(document.createElement('div'))
    const newerLongLine = 'L'.repeat(50)
    await write(term, [
      'before-000',
      'before-111',
      'TARGET',
      'next-line',
      newerLongLine,
      'after-0000',
      'after-1111',
      'after-2222',
      'after-3333',
    ].join('\r\n'))
    term.scrollToLine(findLine(term, 'TARGET'))

    resizeTerminalPreservingViewport(term, 5, 4)

    expect(term.buffer.active.viewportY).toBe(0)
    expect(topLine(term)).toBe('LLLLL')
  })

  it('keeps the physical row for a wrapped line that contains the cursor', async () => {
    const term = new Terminal({ cols: 10, rows: 4, scrollback: 100 })
    term.open(document.createElement('div'))
    const activeLine = `${'a'.repeat(20)}TARGET${'b'.repeat(74)}`
    await write(term, ['before-00', 'before-11', 'before-22', activeLine].join('\r\n'))
    term.scrollToLine(findLineStartingWith(term, 'TARGET'))

    resizeTerminalPreservingViewport(term, 20, 4)

    expect(topLine(term)).toBe('TARGETbbbb')
  })

  it('keeps a live-tail viewport pinned to the bottom', async () => {
    const term = await terminalWithWrappedHistory()
    term.scrollToBottom()

    resizeTerminalPreservingViewport(term, 8, 6)

    expect(term.buffer.active.viewportY).toBe(term.buffer.active.baseY)
  })
})
