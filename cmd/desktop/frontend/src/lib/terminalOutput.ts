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

// tmux splits one process write across %output notifications, while xterm 5
// ignores the synchronized-output markers pi wraps around each redraw. Holding
// only those marked frames keeps their intermediate rows off the renderer.
export class TerminalOutputWriter {
  private prefix = new Uint8Array()
  private frame: Uint8Array[] = []
  private frameBytes = 0
  private endMatched = 0
  private synchronizing = false
  private timer: ReturnType<typeof setTimeout> | undefined

  constructor(private readonly sink: (data: Uint8Array) => void) {}

  write(data: Uint8Array): void {
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
      if (writable) this.sink(retained ? combined.subarray(0, writable) : combined)
      if (retained) {
        this.prefix = combined.slice(writable)
        this.armTimeout()
      }
      return
    }

    if (begin) this.sink(combined.subarray(0, begin))
    this.synchronizing = true
    this.appendFrame(combined.subarray(begin, begin + BEGIN_SYNCHRONIZED_OUTPUT.length))
    this.armTimeout()
    this.writeSynchronized(combined.subarray(begin + BEGIN_SYNCHRONIZED_OUTPUT.length))
  }

  /**
   * Drops whatever is held mid-frame. A resync repaint supersedes the stream
   * these bytes came from, so releasing them afterwards — on the END marker or
   * the timeout — would paint pre-repaint output over the snapshot.
   */
  reset(): void {
    clearTimeout(this.timer)
    this.timer = undefined
    this.prefix = new Uint8Array()
    this.frame = []
    this.frameBytes = 0
    this.endMatched = 0
    this.synchronizing = false
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
      this.flushFrame()
      this.write(data.subarray(i + 1))
      return
    }
    this.appendFrame(data)
    // No END in sight and the frame has grown past what is worth holding —
    // release it rather than keep buffering toward the timeout.
    if (this.frameBytes >= MAX_SYNCHRONIZED_FRAME_BYTES) this.flushFrame()
  }

  private appendFrame(data: Uint8Array): void {
    if (!data.length) return
    this.frame.push(data)
    this.frameBytes += data.length
  }

  private flushFrame(): void {
    clearTimeout(this.timer)
    this.timer = undefined
    const frame = concatenate(this.frame, this.frameBytes)
    this.frame = []
    this.frameBytes = 0
    this.endMatched = 0
    this.synchronizing = false
    if (frame.length) this.sink(frame)
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
        if (frame.length) this.sink(frame)
        return
      }
      const prefix = this.prefix
      this.prefix = new Uint8Array()
      if (prefix.length) this.sink(prefix)
    }, SYNCHRONIZED_OUTPUT_TIMEOUT_MS)
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

function concatenate(chunks: Uint8Array[], size = chunks.reduce((total, chunk) => total + chunk.length, 0)): Uint8Array {
  const result = new Uint8Array(size)
  let offset = 0
  for (const chunk of chunks) {
    result.set(chunk, offset)
    offset += chunk.length
  }
  return result
}
