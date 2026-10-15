import { describe, expect, it } from 'vitest'
import { KittyFilter } from '../kitty/kittyFilter'

const encoder = new TextEncoder()
const decoder = new TextDecoder()
const NONCE = 'abc123'
const OPEN = `\x1b]71137;${NONCE};`

function run(filter: KittyFilter, chunks: string[]): string {
  return chunks.map((chunk) => decoder.decode(filter.rewrite(encoder.encode(chunk)))).join('')
}

function rewrite(...chunks: string[]): string {
  return run(new KittyFilter(NONCE), chunks)
}

// Deterministic, so a failing split can be reproduced.
function randomSplits(data: string, seed: number): string[] {
  const chunks: string[] = []
  let state = seed
  let offset = 0
  while (offset < data.length) {
    state = (state * 1103515245 + 12345) % 2 ** 31
    const size = 1 + (state % 7)
    chunks.push(data.slice(offset, offset + size))
    offset += size
  }
  return chunks
}

const STREAM = [
  'before\x1b[31m',
  '\x1b_Ga=T,f=100,q=2,C=1,c=4,r=2,i=7,m=1;AAAA\x1b\\',
  '\x1b_Gm=0;BBBB\x1b\\',
  '\x1b_Xother apc\x1b\\',
  '\x1b]0;title\x07',
  '\x1b\x1b_Ga=d,d=A,q=2\x1b\\',
  'after\x1b',
].join('')

const EXPECTED = [
  'before\x1b[31m',
  `${OPEN}a=T,f=100,q=2,C=1,c=4,r=2,i=7,m=1;AAAA\x1b\\`,
  `${OPEN}m=0;BBBB\x1b\\`,
  '\x1b_Xother apc\x1b\\',
  '\x1b]0;title\x07',
  `\x1b${OPEN}a=d,d=A,q=2\x1b\\`,
  'after',
].join('')

describe('KittyFilter', () => {
  it('rewrites a graphics APC into the private OSC', () => {
    expect(rewrite('a\x1b_Ga=d,d=a,q=2\x1b\\b')).toBe(`a${OPEN}a=d,d=a,q=2\x1b\\b`)
  })

  it('leaves other APCs and other sequences untouched', () => {
    const data = '\x1b_Xnot graphics\x1b\\\x1b_\x1b\\\x1bP$q"q\x1b\\\x1b[?1049h\x1b]8;;http://x\x07'
    expect(rewrite(data)).toBe(data)
  })

  it('returns the same bytes when there is nothing to rewrite', () => {
    const filter = new KittyFilter(NONCE)
    const data = encoder.encode('plain \x1b[1mtext')
    expect(filter.rewrite(data)).toBe(data)
  })

  it('rewrites a stream split in two at every byte boundary', () => {
    for (let split = 0; split <= STREAM.length; split++) {
      expect(rewrite(STREAM.slice(0, split), STREAM.slice(split)), `split at ${split}`).toBe(EXPECTED)
    }
  })

  it('rewrites a stream delivered one byte at a time', () => {
    expect(rewrite(...STREAM.split(''))).toBe(EXPECTED)
  })

  it('rewrites a stream split across many uneven chunks', () => {
    for (let seed = 1; seed <= 50; seed++) {
      expect(rewrite(...randomSplits(STREAM, seed)), `seed ${seed}`).toBe(EXPECTED)
    }
  })

  it('holds a trailing ESC until the next chunk says what it opens', () => {
    const filter = new KittyFilter(NONCE)
    expect(run(filter, ['text\x1b'])).toBe('text')
    expect(run(filter, ['[0m'])).toBe('\x1b[0m')
  })

  // BEL ends an OSC but not an APC, so letting it through would cut the
  // command short where a real Kitty terminal keeps reading.
  it('drops BEL inside a command', () => {
    expect(rewrite('\x1b_Gi=1;AA\x07AA\x1b\\')).toBe(`${OPEN}i=1;AAAA\x1b\\`)
  })

  // xterm dispatches an OSC that an ESC interrupts; an APC is abandoned.
  it('aborts a command an ESC interrupts with CAN and keeps the new sequence', () => {
    expect(rewrite('\x1b_Gi=1;AA\x1b[31mred')).toBe(`${OPEN}i=1;AA\x18\x1b[31mred`)
  })

  it('passes CAN through so it aborts the OSC as it would the APC', () => {
    expect(rewrite('\x1b_Gi=1;AA\x18after')).toBe(`${OPEN}i=1;AA\x18after`)
  })

  it('drops a command past the size bound and resumes after its terminator', () => {
    const payload = 'A'.repeat((16 << 20) + 10)
    const filter = new KittyFilter(NONCE)
    const out = run(filter, [`x\x1b_Gi=1;${payload.slice(0, 1 << 20)}`, payload.slice(1 << 20), '\x1b\\y'])
    expect(out.startsWith(`x${OPEN}i=1;`)).toBe(true)
    expect(out.endsWith('\x18y')).toBe(true)
    expect(out.length).toBeLessThan((16 << 20) + 64)
  })

  it('forgets a partial sequence on reset', () => {
    const filter = new KittyFilter(NONCE)
    run(filter, ['\x1b_Gi=1;AA'])
    filter.reset()
    expect(run(filter, ['plain'])).toBe('plain')

    run(filter, ['\x1b_'])
    filter.reset()
    expect(run(filter, ['Gx'])).toBe('Gx')
  })
})
