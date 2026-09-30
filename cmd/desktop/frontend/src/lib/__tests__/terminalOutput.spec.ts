import { afterEach, describe, expect, it, vi } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { TerminalOutputWriter } from '../terminalOutput'

const encoder = new TextEncoder()
const decoder = new TextDecoder()

function write(term: Terminal, data: string | Uint8Array): Promise<void> {
  return new Promise((resolve) => term.write(data, resolve))
}

function screen(term: Terminal): string[] {
  const buffer = term.buffer.active
  return Array.from({ length: term.rows }, (_, row) =>
    buffer.getLine(buffer.baseY + row)?.translateToString(true) ?? '')
}

describe('TerminalOutputWriter', () => {
  afterEach(() => vi.useRealTimers())

  it('keeps a pi redraw off xterm until tmux delivers its synchronized end', async () => {
    const initial = ['old response', '', '> prompt', 'footer'].join('\r\n')
    const firstFragment = encoder.encode([
      '\x1b[?2026h',
      '\x1b[3A\r',
      '\x1b[2Knew response\r\n',
      '\x1b[2Knew detail\r\n',
      '\x1b[2K',
    ].join(''))
    const raw = new Terminal({ cols: 24, rows: 4, scrollback: 100 })
    await write(raw, initial)
    await write(raw, firstFragment)
    expect(screen(raw)).toEqual(['new response', 'new detail', '', 'footer'])

    const term = new Terminal({ cols: 24, rows: 4, scrollback: 100 })
    await write(term, initial)
    const before = screen(term)
    const writes: Promise<void>[] = []
    const output = new TerminalOutputWriter((data) => { writes.push(write(term, data)) })

    output.write(firstFragment)

    expect(writes).toHaveLength(0)
    expect(screen(term)).toEqual(before)

    output.write(encoder.encode([
      '> prompt\r\n',
      '\x1b[2Kfooter',
      '\x1b[?2026l',
    ].join('')))
    await Promise.all(writes)

    expect(writes).toHaveLength(1)
    expect(screen(term)).toEqual(['new response', 'new detail', '> prompt', 'footer'])
  })

  it('recognizes synchronized markers split at every byte boundary', () => {
    const chunks: Uint8Array[] = []
    const output = new TerminalOutputWriter((data) => { chunks.push(data) })
    const frame = encoder.encode('\x1b[?2026h\r\x1b[2Kworking\x1b[?2026l')

    for (const byte of frame) output.write(Uint8Array.of(byte))

    expect(chunks).toHaveLength(1)
    expect(decoder.decode(chunks[0])).toBe('\x1b[?2026h\r\x1b[2Kworking\x1b[?2026l')
  })

  it('passes ordinary and alternate-screen output through unchanged', () => {
    const chunks: Uint8Array[] = []
    const output = new TerminalOutputWriter((data) => { chunks.push(data) })
    const data = encoder.encode('\x1b[?1049hvim\x1b[?1049l')

    output.write(data)

    expect(chunks).toHaveLength(1)
    expect(decoder.decode(chunks[0])).toBe('\x1b[?1049hvim\x1b[?1049l')
  })

  it('preserves bytes around consecutive synchronized redraws', () => {
    const chunks: Uint8Array[] = []
    const output = new TerminalOutputWriter((data) => { chunks.push(data) })
    const data = 'before\x1b[?2026hfirst\x1b[?2026lbetween\x1b[?2026hsecond\x1b[?2026lafter'

    output.write(encoder.encode(data))

    expect(chunks.map((chunk) => decoder.decode(chunk))).toEqual([
      'before', '\x1b[?2026hfirst\x1b[?2026l', 'between', '\x1b[?2026hsecond\x1b[?2026l', 'after',
    ])
  })

  // A resync repaint supersedes the stream a held frame came from, so releasing
  // it afterwards — on its END marker or on the timeout — would paint
  // pre-repaint output over the snapshot.
  it('drops a held frame on reset instead of releasing it behind a repaint', () => {
    vi.useFakeTimers()
    const chunks: Uint8Array[] = []
    const output = new TerminalOutputWriter((data) => { chunks.push(data) })

    output.write(encoder.encode('\x1b[?2026hpartial'))
    output.reset()

    output.write(encoder.encode('REPAINTED'))
    vi.advanceTimersByTime(1000)

    expect(chunks.map((chunk) => decoder.decode(chunk))).toEqual(['REPAINTED'])
  })

  it('releases an unterminated redraw instead of stalling the terminal', () => {
    vi.useFakeTimers()
    const chunks: Uint8Array[] = []
    const output = new TerminalOutputWriter((data) => { chunks.push(data) })

    output.write(encoder.encode('\x1b[?2026hpartial'))
    expect(chunks).toHaveLength(0)

    vi.advanceTimersByTime(1000)

    expect(chunks).toHaveLength(1)
    expect(decoder.decode(chunks[0])).toBe('\x1b[?2026hpartial')
  })
})

// A program that opens a synchronized frame and then streams without ever
// closing it must not pin its output until the timeout: that is unbounded
// memory and a pane that looks frozen for a full second.
it('releases a synchronized frame that grows past the hold bound', () => {
  const written: Uint8Array[] = []
  const writer = new TerminalOutputWriter((data) => written.push(data))

  writer.write(Uint8Array.from([0x1b, 0x5b, 0x3f, 0x32, 0x30, 0x32, 0x36, 0x68]))
  expect(written).toHaveLength(0)

  let sent = 0
  const chunk = new Uint8Array(64 * 1024).fill(0x61)
  while (written.length === 0 && sent < 4 << 20) {
    writer.write(chunk)
    sent += chunk.length
  }

  expect(written.length).toBeGreaterThan(0)
  expect(sent).toBeLessThanOrEqual(2 << 20)
  const total = written.reduce((n, part) => n + part.length, 0)
  expect(total).toBe(sent + 8)
})
