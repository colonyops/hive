import { describe, expect, it } from 'vitest'
import { splitForWrite } from '../terminalOutput'

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
