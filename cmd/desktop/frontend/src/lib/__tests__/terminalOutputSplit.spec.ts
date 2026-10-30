import { describe, expect, it, vi } from 'vitest'
import { PacedWriter, splitForWrite } from '../terminalOutput'

describe('splitForWrite', () => {
  it('leaves a normal chunk whole', () => {
    const data = new Uint8Array(1024)
    expect(splitForWrite(data)).toEqual([data])
  })

  it('cuts a snapshot into pieces that rebuild it byte for byte', () => {
    const data = Uint8Array.from({ length: 100_000 }, (_, i) => i % 251)
    const pieces = splitForWrite(data)
    expect(pieces.length).toBeGreaterThan(1)
    for (const piece of pieces) expect(piece.length).toBeLessThanOrEqual(32 << 10)
    const rebuilt = new Uint8Array(data.length)
    let offset = 0
    for (const piece of pieces) {
      rebuilt.set(piece, offset)
      offset += piece.length
    }
    expect(rebuilt).toEqual(data)
  })
})

describe('PacedWriter', () => {
  function fakeTerm() {
    const writes: { data: Uint8Array; done?: () => void }[] = []
    return {
      writes,
      term: {
        reset: vi.fn(),
        write: (data: string | Uint8Array, done?: () => void) => void writes.push({ data: data as Uint8Array, done }),
      },
    }
  }
  const big = new Uint8Array((32 << 10) * 2 + 1)

  it('asks for no callback on a small frame nobody is waiting on', () => {
    const { term, writes } = fakeTerm()
    new PacedWriter(term, null).write(new Uint8Array(10), false)
    expect(writes[0].done).toBeUndefined()
  })

  it('hands xterm a split frame one piece at a time, and tells the caller after the last', () => {
    const { term, writes } = fakeTerm()
    const whenParsed = vi.fn()
    const parsed: number[] = []
    new PacedWriter(term, (bytes) => parsed.push(bytes)).write(big, false, whenParsed)

    expect(writes).toHaveLength(1)
    writes[0].done?.()
    expect(writes).toHaveLength(2)
    writes[1].done?.()
    expect(writes).toHaveLength(3)
    expect(whenParsed).not.toHaveBeenCalled()
    writes[2].done?.()
    expect(whenParsed).toHaveBeenCalledOnce()
    expect(parsed).toEqual([32 << 10, 32 << 10, 1])
  })

  it('keeps later output behind the pieces still waiting', () => {
    const { term, writes } = fakeTerm()
    const writer = new PacedWriter(term, null)
    writer.write(big, false)
    const later = new Uint8Array(1)
    writer.write(later, false)
    for (let i = 0; i < 3; i++) writes[i].done?.()
    expect(writes[3].data).toBe(later)
  })

  it('keeps an atomic frame in one write', () => {
    const { term, writes } = fakeTerm()
    new PacedWriter(term, null).write(big, true)
    expect(writes).toHaveLength(1)
    expect(writes[0].data).toBe(big)
  })

  it('takes back what xterm has not been handed and carries on from a clean queue', () => {
    const { term, writes } = fakeTerm()
    const writer = new PacedWriter(term, null)
    writer.write(big, false)
    expect(writer.discard()).toBe((32 << 10) + 1)
    writes[0].done?.()
    expect(writes).toHaveLength(1)

    writer.write(new Uint8Array(1), false)
    expect(writes).toHaveLength(2)
  })
})
