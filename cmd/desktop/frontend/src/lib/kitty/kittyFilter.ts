export const KITTY_OSC = 71137

const ESC = 0x1b
const BEL = 0x07
const CAN = 0x18
const SUB = 0x1a
const UNDERSCORE = 0x5f
const UPPERCASE_G = 0x47
const BACKSLASH = 0x5c
const ABORT = Uint8Array.of(CAN)
const EMPTY = new Uint8Array()

const MAX_SEQUENCE_BYTES = 16 << 20

type State = 'ground' | 'escape' | 'apc' | 'kitty' | 'kittyEscape' | 'discard' | 'discardEscape'

/**
 * Rewrites Kitty graphics commands, `ESC _ G <payload> ESC \`, into
 * `ESC ] 71137 ; <nonce> ; <payload> ESC \` so an OSC handler sees them at
 * their exact place in the stream. xterm 6.0 drops every APC and has no hook
 * for one.
 *
 * Bytes stream through rather than being buffered: only a trailing `ESC` or
 * `ESC _` is held until the next chunk says what it opens. Inside a command
 * the filter keeps xterm's OSC parser in step with an APC: BEL is dropped
 * (it would end an OSC but not an APC), and an ESC that is not ST aborts with
 * CAN first (xterm would otherwise dispatch the truncated OSC).
 */
export class KittyFilter {
  private state: State = 'ground'
  private held = EMPTY
  private size = 0
  private readonly opener: Uint8Array

  constructor(nonce: string) {
    this.opener = new TextEncoder().encode(`\x1b]${KITTY_OSC};${nonce};`)
  }

  rewrite(input: Uint8Array): Uint8Array {
    const data = this.held.length ? concatenate([this.held, input]) : input
    this.held = EMPTY
    const out: Uint8Array[] = []
    let run = this.state === 'discard' ? -1 : 0
    const keep = (end: number) => {
      if (run === -1 || end <= run) return
      out.push(run === 0 && end === data.length ? data : data.subarray(run, end))
    }

    for (let i = 0; i < data.length; i++) {
      const byte = data[i]
      switch (this.state) {
        case 'ground':
          if (byte === ESC) this.state = 'escape'
          break
        case 'escape':
          this.state = byte === UNDERSCORE ? 'apc' : byte === ESC ? 'escape' : 'ground'
          break
        case 'apc':
          if (byte === UPPERCASE_G) {
            keep(i - 2)
            out.push(this.opener)
            run = i + 1
            this.size = 0
            this.state = 'kitty'
          } else {
            this.state = byte === ESC ? 'escape' : 'ground'
          }
          break
        case 'kitty':
          if (byte === ESC) {
            this.state = 'kittyEscape'
          } else if (byte === BEL) {
            keep(i)
            run = i + 1
          } else if (byte === CAN || byte === SUB) {
            this.state = 'ground'
          } else if (++this.size > MAX_SEQUENCE_BYTES) {
            keep(i)
            out.push(ABORT)
            run = -1
            this.state = 'discard'
          }
          break
        case 'kittyEscape':
          if (byte === BACKSLASH) {
            this.state = 'ground'
          } else {
            keep(i - 1)
            out.push(ABORT)
            run = i - 1
            this.state = 'escape'
            i--
          }
          break
        case 'discard':
          if (byte === ESC) {
            this.state = 'discardEscape'
          } else if (byte === CAN || byte === SUB) {
            run = i + 1
            this.state = 'ground'
          }
          break
        case 'discardEscape':
          if (byte === BACKSLASH) {
            run = i + 1
            this.state = 'ground'
          } else {
            run = i - 1
            this.state = 'escape'
            i--
          }
          break
      }
    }

    const heldBytes = this.holdTail()
    if (heldBytes) this.held = data.slice(data.length - heldBytes)
    keep(data.length - heldBytes)
    if (out.length === 1) return out[0]
    return concatenate(out)
  }

  reset(): void {
    this.state = 'ground'
    this.held = EMPTY
    this.size = 0
  }

  // A held prefix is scanned again with the next chunk, so the state rewinds
  // to where it stood before those bytes.
  private holdTail(): number {
    switch (this.state) {
      case 'escape':
        this.state = 'ground'
        return 1
      case 'apc':
        this.state = 'ground'
        return 2
      case 'kittyEscape':
        this.state = 'kitty'
        return 1
      case 'discardEscape':
        this.state = 'discard'
        return 1
      default:
        return 0
    }
  }
}

function concatenate(chunks: Uint8Array[]): Uint8Array {
  const result = new Uint8Array(chunks.reduce((total, chunk) => total + chunk.length, 0))
  let offset = 0
  for (const chunk of chunks) {
    result.set(chunk, offset)
    offset += chunk.length
  }
  return result
}
