import { bench, describe } from 'vitest'
import { decodeFrame } from '../terminalClient'
import { ScreenTitleFilter, TerminalOutputWriter } from '../terminalOutput'
import { KittyFilter } from '../kitty/kittyFilter'

const encoder = new TextEncoder()

function repeatTo(size: number, text: string): Uint8Array {
  return encoder.encode(text.repeat(Math.ceil(size / text.length)).slice(0, size))
}

// One frame of what the broker delivers under a flood.
const FRAME = 64 << 10
const plain = repeatTo(FRAME, 'the quick brown fox jumps over the lazy dog 0123456789 abcdefghij\r\n')
const colored = repeatTo(
  FRAME,
  '\x1b[32m0001234\x1b[0m \x1b[1;33mthe quick\x1b[0m brown \x1b[4mfox\x1b[0m jumps \x1b[38;5;120m0123456789\x1b[0m\r\n',
)

function outputFrame(data: Uint8Array): Uint8Array {
  const header = [0, 2, ...encoder.encode('@1'), 2, ...encoder.encode('%1')]
  const frame = new Uint8Array(header.length + data.length)
  frame.set(header)
  frame.set(data, header.length)
  return frame
}

describe('one 64 KiB frame through the pane filters', () => {
  const discard = (): void => {}
  for (const [name, data] of [
    ['plain', plain],
    ['colored', colored],
  ] as const) {
    bench(`writer ${name}`, () => {
      new TerminalOutputWriter(discard, new KittyFilter('nonce')).write(data)
    })
    bench(`screen title ${name}`, () => {
      new ScreenTitleFilter().strip(data)
    })
    bench(`kitty ${name}`, () => {
      new KittyFilter('nonce').rewrite(data)
    })
  }
})

describe('frame decode', () => {
  const frame = outputFrame(plain)
  bench('decodeFrame', () => {
    decodeFrame(frame)
  })
})
