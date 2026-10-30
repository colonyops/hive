import type { IDisposable, Terminal } from '@xterm/xterm'
import { perfEnabled, usePerf } from '../composables/usePerf'
import { collectFrames, round, type FrameSummary } from './frameCollector'

const perf = usePerf('terminal')

// A pause this long ends a burst of output. Shorter would split one `cat` on the
// gaps tmux leaves between reads.
const BURST_IDLE_MS = 200
// Below this a burst is a prompt or an echo, which `input:echo` already covers,
// and one span per prompt would push the interesting ones out of the file.
const BURST_MIN_BYTES = 32 << 10
const SCROLL_IDLE_MS = 150
const ECHO_GIVE_UP_MS = 2000

/**
 * Times what the person at one pane feels: a key to its echo on screen, a burst
 * of output, a scroll gesture. It writes spans at human rates, never per chunk,
 * so it can stay on for a whole dev session (ADR
 * ui-performance-spans-are-recorded-to-jsonl).
 */
export interface PaneProbe extends IDisposable {
  /** Bytes the pane is about to send, as onData gave them. */
  input(data: string): void
  /** Output off the socket, before anything filters it. */
  received(bytes: number): void
  /** Set when the writer should report parsed bytes: null means the probe is off. */
  readonly parsed: ((bytes: number) => void) | null
  /** The bytes off the socket that the emulator has not parsed yet. */
  backlog(): number
  /** Every output byte off the socket so far. */
  totalReceived(): number
  /** Call after term.open(): the wheel listener needs the element. */
  opened(): void
}

const OFF: PaneProbe = {
  input: () => {},
  received: () => {},
  parsed: null,
  backlog: () => 0,
  totalReceived: () => 0,
  opened: () => {},
  dispose: () => {},
}

interface PendingEcho {
  at: number
  arrivedAt: number
  parsedAt: number
  // The total parsed count that means the frame carrying the echo is in the grid.
  target: number
  busy: boolean
}

interface Burst {
  startedAt: number
  lastAt: number
  bytes: number
  socketFrames: number
  maxBacklog: number
  visible: boolean
  frames: (() => FrameSummary) | null
}

interface ScrollGesture {
  startedAt: number
  lastAt: number
  events: number
  firstRenderMs: number
  startY: number
  frames: () => FrameSummary
}

function isMouseReport(data: string): boolean {
  return data.startsWith('\x1b[<') || data.startsWith('\x1b[M')
}

export function probePane(paneId: string, term: Terminal, renderer: () => boolean): PaneProbe {
  if (!perfEnabled()) return OFF

  let received = 0
  let parsed = 0
  let echo: PendingEcho | null = null
  let burst: Burst | null = null
  let burstTimer: ReturnType<typeof setTimeout> | undefined
  let gesture: ScrollGesture | null = null
  let gestureTimer: ReturnType<typeof setTimeout> | undefined
  let wheelTarget: HTMLElement | null = null

  const visible = (): boolean => !!term.element?.offsetParent

  function closeBurst(): void {
    if (!burst) return
    const done = burst
    burst = null
    const frames = done.frames?.()
    if (done.bytes < BURST_MIN_BYTES) return
    const durationMs = Math.max(1, done.lastAt - done.startedAt)
    perf.record('output:burst', durationMs, {
      pane: paneId,
      bytes: done.bytes,
      socketFrames: done.socketFrames,
      mbPerSec: round(done.bytes / 1e6 / (durationMs / 1000)),
      maxBacklog: done.maxBacklog,
      visible: done.visible,
      webgl: renderer(),
      ...frames,
    })
  }

  function watchBurst(): void {
    burstTimer = setTimeout(() => {
      burstTimer = undefined
      if (!burst) return
      if (received > parsed || performance.now() - burst.lastAt < BURST_IDLE_MS) watchBurst()
      else closeBurst()
    }, BURST_IDLE_MS)
  }

  function closeGesture(): void {
    if (!gesture) return
    const done = gesture
    gesture = null
    perf.record('scroll:gesture', done.lastAt - done.startedAt, {
      pane: paneId,
      events: done.events,
      lines: Math.abs(term.buffer.active.viewportY - done.startY),
      firstRenderMs: round(done.firstRenderMs),
      mouseMode: term.modes.mouseTrackingMode !== 'none',
      altScreen: term.buffer.active.type === 'alternate',
      webgl: renderer(),
      ...done.frames(),
    })
  }

  function onWheel(): void {
    const now = performance.now()
    gesture ??= {
      startedAt: now,
      lastAt: now,
      events: 0,
      firstRenderMs: -1,
      startY: term.buffer.active.viewportY,
      frames: collectFrames(),
    }
    gesture.events++
    gesture.lastAt = now
    clearTimeout(gestureTimer)
    gestureTimer = setTimeout(closeGesture, SCROLL_IDLE_MS)
  }

  const onRender = term.onRender(() => {
    const now = performance.now()
    if (gesture && gesture.firstRenderMs < 0) gesture.firstRenderMs = now - gesture.startedAt
    if (!echo || !echo.parsedAt) return
    const done = echo
    echo = null
    perf.record('input:echo', now - done.at, {
      pane: paneId,
      wireMs: round(done.arrivedAt - done.at),
      parseMs: round(done.parsedAt - done.arrivedAt),
      renderMs: round(now - done.parsedAt),
      busy: done.busy,
      webgl: renderer(),
    })
  })

  return {
    input(data) {
      if (isMouseReport(data)) return
      const now = performance.now()
      if (echo && now - echo.at > ECHO_GIVE_UP_MS) echo = null
      echo ??= { at: now, arrivedAt: 0, parsedAt: 0, target: 0, busy: received > parsed }
    },
    received(bytes) {
      const now = performance.now()
      received += bytes
      if (echo && !echo.arrivedAt) {
        echo.arrivedAt = now
        echo.target = received
      }
      if (!burst) {
        const shown = visible()
        burst = {
          startedAt: now,
          lastAt: now,
          bytes: 0,
          socketFrames: 0,
          maxBacklog: 0,
          visible: shown,
          frames: shown ? collectFrames() : null,
        }
        if (!burstTimer) watchBurst()
      }
      burst.lastAt = now
      burst.bytes += bytes
      burst.socketFrames++
      burst.maxBacklog = Math.max(burst.maxBacklog, received - parsed)
    },
    parsed(bytes) {
      parsed += bytes
      const now = performance.now()
      if (burst) burst.lastAt = now
      if (echo && echo.arrivedAt && !echo.parsedAt && parsed >= echo.target) echo.parsedAt = now
    },
    backlog: () => received - parsed,
    totalReceived: () => received,
    opened() {
      wheelTarget = term.element ?? null
      wheelTarget?.addEventListener('wheel', onWheel, { capture: true, passive: true })
    },
    dispose() {
      clearTimeout(burstTimer)
      clearTimeout(gestureTimer)
      closeBurst()
      closeGesture()
      onRender.dispose()
      wheelTarget?.removeEventListener('wheel', onWheel, { capture: true })
    },
  }
}

/** A pane the benchmark driver can reach; see lib/perf/terminalBench.ts. */
export interface BenchPane {
  paneId: string
  term: Terminal
  /** Bytes to the pane as if typed, bypassing the keyboard. */
  send(data: string): void
  backlog(): number
  /** Output bytes off the socket so far, to see what a switch makes a TUI redraw. */
  received(): number
  webgl(): boolean
  /** Drops the stream and every emulator, then attaches again, resolving at first paint. */
  reattach(): Promise<void>
  /** Selects the session's next window, resolving once it is on screen. */
  nextWindow(): Promise<void>
}

const registeredPanes = new Map<string, BenchPane>()

export function registerBenchPane(pane: BenchPane): IDisposable {
  if (!perfEnabled()) return { dispose: () => {} }
  registeredPanes.set(pane.paneId, pane)
  return { dispose: () => registeredPanes.delete(pane.paneId) }
}

export function visibleBenchPane(): BenchPane | undefined {
  return [...registeredPanes.values()].find((pane) => pane.term.element?.offsetParent)
}

export function benchPanes(): BenchPane[] {
  return [...registeredPanes.values()]
}
