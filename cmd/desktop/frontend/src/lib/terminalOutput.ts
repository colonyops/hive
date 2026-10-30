import type { Terminal } from '@xterm/xterm'

const BEGIN_SYNCHRONIZED_OUTPUT = Uint8Array.from([0x1b, 0x5b, 0x3f, 0x32, 0x30, 0x32, 0x36, 0x68])
const END_SYNCHRONIZED_OUTPUT = Uint8Array.from([0x1b, 0x5b, 0x3f, 0x32, 0x30, 0x32, 0x36, 0x6c])
const SYNCHRONIZED_OUTPUT_TIMEOUT_MS = 1000
// A held frame is output the pane has produced and the renderer has not been
// shown. Only an END marker or the timeout releases one, so a program that
// opens a frame and then streams without closing it would otherwise pin every
// byte it writes for a full second — unbounded memory, and a pane that appears
// frozen. Past this the frame is released mid-redraw: one torn repaint beats
// holding megabytes and a second of latency.
const MAX_SYNCHRONIZED_FRAME_BYTES = 1 << 20

// xterm yields to the browser only between writes, so one write of a pane's whole
// scrollback snapshot held the main thread for up to 220ms on a real session.
const MAX_WRITE_BYTES = 32 << 10

/**
 * Cuts output into pieces xterm can yield between. A cut can land inside an escape
 * sequence or a UTF-8 character: xterm's parser keeps its state across writes.
 */
export function splitForWrite(data: Uint8Array): Uint8Array[] {
  if (data.length <= MAX_WRITE_BYTES) return [data]
  const pieces: Uint8Array[] = []
  for (let offset = 0; offset < data.length; offset += MAX_WRITE_BYTES) {
    pieces.push(data.subarray(offset, offset + MAX_WRITE_BYTES))
  }
  return pieces
}

interface QueuedPiece {
  data: Uint8Array
  whenParsed?: () => void
}

/**
 * Hands xterm a split frame one piece at a time, each from the last one's parsed
 * callback, so what a repaint supersedes stays here where `clear` can drop it.
 */
export class PacedWriter {
  private queue: QueuedPiece[] = []
  private writing = false

  constructor(
    private readonly term: Pick<Terminal, 'write' | 'reset'>,
    private readonly reportParsed: ((bytes: number) => void) | null,
  ) {}

  /** `atomic` keeps the frame in one write, so nothing can draw between its parts. */
  write(data: Uint8Array, atomic: boolean, whenParsed?: () => void): void {
    const pieces = atomic ? [data] : splitForWrite(data)
    if (pieces.length === 1 && !this.writing) {
      if (!whenParsed && !this.reportParsed) {
        this.term.write(data)
        return
      }
      this.term.write(data, () => {
        this.reportParsed?.(data.length)
        whenParsed?.()
      })
      return
    }
    pieces.forEach((piece, index) => {
      this.queue.push({ data: piece, whenParsed: index === pieces.length - 1 ? whenParsed : undefined })
    })
    if (!this.writing) this.next()
  }

  /** Drops what xterm has not been handed and reports how many bytes that was. */
  discard(): number {
    const bytes = this.queue.reduce((total, piece) => total + piece.data.length, 0)
    this.queue = []
    return bytes
  }

  /**
   * Readies the emulator for a repaint and reports the bytes dropped. `term.reset()`
   * leaves xterm's own queue alone, so calling it directly would land what is queued
   * on the cleared screen, and a piece cut inside an escape would eat the repaint's
   * first bytes. The reset therefore goes through that queue, behind a CAN that ends
   * any such sequence.
   */
  clear(whenCleared: () => void): number {
    const bytes = this.discard()
    this.term.write('\x18', () => {
      this.term.reset()
      whenCleared()
    })
    return bytes
  }

  private next(): void {
    const piece = this.queue.shift()
    this.writing = piece !== undefined
    if (!piece) return
    this.term.write(piece.data, () => {
      this.reportParsed?.(piece.data.length)
      piece.whenParsed?.()
      this.next()
    })
  }
}

/** A stateful rewrite of the pane's bytes that runs ahead of the writer's own filters. */
export interface StreamFilter {
  rewrite(data: Uint8Array): Uint8Array
  reset(): void
}

// tmux splits one process write across %output notifications. xterm 6 defers
// its own row renders inside the synchronized-output markers pi wraps around
// each redraw, but it still parses each part as it arrives, and the Kitty layer
// paints from the buffer on its own animation frame. Holding only those marked
// frames keeps the buffer, and so the images, from showing a half-drawn one.
export class TerminalOutputWriter {
  private prefix = new Uint8Array()
  private frame: Uint8Array[] = []
  private frameBytes = 0
  private endMatched = 0
  private synchronizing = false
  private timer: ReturnType<typeof setTimeout> | undefined
  private readonly titles = new ScreenTitleFilter()

  constructor(
    private readonly sink: (data: Uint8Array, atomic: boolean) => void,
    private readonly filter?: StreamFilter,
  ) {}

  /** Reports the bytes the filters let through, which is what will reach the sink. */
  write(data: Uint8Array): number {
    const kept = this.titles.strip(this.filter ? this.filter.rewrite(data) : data)
    this.forward(kept)
    return kept.length
  }

  private forward(data: Uint8Array): void {
    if (!data.length) return
    if (this.synchronizing) {
      this.writeSynchronized(data)
      return
    }

    const combined = this.prefix.length ? concatenate([this.prefix, data]) : data
    if (this.prefix.length) {
      clearTimeout(this.timer)
      this.timer = undefined
      this.prefix = new Uint8Array()
    }
    const begin = indexOf(combined, BEGIN_SYNCHRONIZED_OUTPUT)
    if (begin === -1) {
      const retained = matchingSuffixLength(combined, BEGIN_SYNCHRONIZED_OUTPUT)
      const writable = combined.length - retained
      if (writable) this.sink(retained ? combined.subarray(0, writable) : combined, false)
      if (retained) {
        this.prefix = combined.slice(writable)
        this.armTimeout()
      }
      return
    }

    if (begin) this.sink(combined.subarray(0, begin), false)
    this.synchronizing = true
    this.appendFrame(combined.subarray(begin, begin + BEGIN_SYNCHRONIZED_OUTPUT.length))
    this.armTimeout()
    this.writeSynchronized(combined.subarray(begin + BEGIN_SYNCHRONIZED_OUTPUT.length))
  }

  /**
   * Drops whatever is held mid-frame. A resync repaint supersedes the stream
   * these bytes came from, so releasing them afterwards — on the END marker or
   * the timeout — would paint pre-repaint output over the snapshot. Reports how
   * many bytes that was.
   */
  reset(): number {
    const held = this.prefix.length + this.frameBytes
    this.filter?.reset()
    this.titles.reset()
    clearTimeout(this.timer)
    this.timer = undefined
    this.prefix = new Uint8Array()
    this.frame = []
    this.frameBytes = 0
    this.endMatched = 0
    this.synchronizing = false
    return held
  }

  dispose(): void {
    this.reset()
  }

  private writeSynchronized(data: Uint8Array): void {
    for (let i = 0; i < data.length; i++) {
      const byte = data[i]
      if (byte === END_SYNCHRONIZED_OUTPUT[this.endMatched]) this.endMatched++
      else this.endMatched = byte === END_SYNCHRONIZED_OUTPUT[0] ? 1 : 0
      if (this.endMatched !== END_SYNCHRONIZED_OUTPUT.length) continue

      this.appendFrame(data.subarray(0, i + 1))
      this.flushFrame(true)
      this.forward(data.subarray(i + 1))
      return
    }
    this.appendFrame(data)
    // No END in sight and the frame has grown past what is worth holding —
    // release it rather than keep buffering toward the timeout.
    if (this.frameBytes >= MAX_SYNCHRONIZED_FRAME_BYTES) this.flushFrame(false)
  }

  private appendFrame(data: Uint8Array): void {
    if (!data.length) return
    this.frame.push(data)
    this.frameBytes += data.length
  }

  private flushFrame(complete: boolean): void {
    clearTimeout(this.timer)
    this.timer = undefined
    const frame = concatenate(this.frame, this.frameBytes)
    this.frame = []
    this.frameBytes = 0
    this.endMatched = 0
    this.synchronizing = false
    if (frame.length) this.sink(frame, complete)
  }

  private armTimeout(): void {
    if (this.timer) return
    this.timer = setTimeout(() => {
      this.timer = undefined
      if (this.synchronizing) {
        const frame = concatenate(this.frame, this.frameBytes)
        this.frame = []
        this.frameBytes = 0
        this.endMatched = 0
        this.synchronizing = false
        if (frame.length) this.sink(frame, false)
        return
      }
      const prefix = this.prefix
      this.prefix = new Uint8Array()
      if (prefix.length) this.sink(prefix, false)
    }, SYNCHRONIZED_OUTPUT_TIMEOUT_MS)
  }
}

const ESC = 0x1b
const CAN = 0x18
const SUB = 0x1a
const LOWERCASE_K = 0x6b
const BACKSLASH = 0x5c
const LONE_ESC = Uint8Array.of(ESC)

/**
 * Removes screen's window-name string, `ESC k <name> ESC \`.
 *
 * Shells send it when TERM is screen* or tmux* (oh-my-zsh does by default).
 * tmux consumes it on the pane but control mode forwards the raw bytes, and
 * xterm.js has no handler for it, so it prints the name. The string can span
 * %output notifications, so the state carries across chunks; an ESC that ends
 * a chunk is held until the next byte says whether it opens one.
 *
 * The string ends the way tmux ends it, so the pane matches tmux's grid: ST
 * ends it, CAN or SUB abort it, and any other ESC ends it and starts a new
 * sequence. BEL does not end it.
 */
export class ScreenTitleFilter {
  private state: 'ground' | 'escape' | 'title' | 'titleEscape' = 'ground'

  strip(data: Uint8Array): Uint8Array {
    const kept: Uint8Array[] = []
    // Start of the bytes being passed through, or -1 while dropping or while
    // the ESC that opened this.state came from an earlier chunk.
    let run = this.state === 'ground' ? 0 : -1
    for (let i = 0; i < data.length; i++) {
      const byte = data[i]
      switch (this.state) {
        case 'ground':
          if (byte === ESC) this.state = 'escape'
          break
        case 'escape':
          if (byte === LOWERCASE_K) {
            if (run !== -1 && i - 1 > run) kept.push(data.subarray(run, i - 1))
            run = -1
            this.state = 'title'
          } else {
            if (run === -1) {
              kept.push(LONE_ESC)
              run = i
            }
            this.state = byte === ESC ? 'escape' : 'ground'
          }
          break
        case 'title':
          if (byte === ESC) this.state = 'titleEscape'
          else if (byte === CAN || byte === SUB) {
            this.state = 'ground'
            run = i
          }
          break
        case 'titleEscape':
          if (byte === BACKSLASH) {
            this.state = 'ground'
            run = i + 1
          } else {
            this.state = 'escape'
            i--
          }
          break
      }
    }
    if (run !== -1) {
      const end = this.state === 'escape' ? data.length - 1 : data.length
      if (end > run) kept.push(data.subarray(run, end))
    }
    if (kept.length === 1) return kept[0]
    return concatenate(kept)
  }

  reset(): void {
    this.state = 'ground'
  }
}

function indexOf(data: Uint8Array, sequence: Uint8Array): number {
  outer: for (let i = 0; i <= data.length - sequence.length; i++) {
    for (let j = 0; j < sequence.length; j++) {
      if (data[i + j] !== sequence[j]) continue outer
    }
    return i
  }
  return -1
}

function matchingSuffixLength(data: Uint8Array, sequence: Uint8Array): number {
  const max = Math.min(data.length, sequence.length - 1)
  for (let length = max; length > 0; length--) {
    let matches = true
    for (let i = 0; i < length; i++) {
      if (data[data.length - length + i] !== sequence[i]) {
        matches = false
        break
      }
    }
    if (matches) return length
  }
  return 0
}

function concatenate(
  chunks: Uint8Array[],
  size = chunks.reduce((total, chunk) => total + chunk.length, 0),
): Uint8Array {
  const result = new Uint8Array(size)
  let offset = 0
  for (const chunk of chunks) {
    result.set(chunk, offset)
    offset += chunk.length
  }
  return result
}
