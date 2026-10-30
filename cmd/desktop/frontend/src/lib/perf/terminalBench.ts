import type { Terminal } from '@xterm/xterm'
import { flushPerf, usePerf } from '../../composables/usePerf'
import { collectFrames, percentile, round, type FrameSummary } from '../frameCollector'
import { visibleBenchPane, type BenchPane } from '../terminalPerf'

/**
 * Scripted workloads against whichever real pane is on screen, run inside the
 * page so the same code measures Chromium in the Docker lane and the WKWebView
 * in the native app. Each one drives the actual stack: the shell runs in tmux
 * and its bytes come back through the control client and the socket.
 */

const perf = usePerf('bench')
const LINE_TEXT = 'the quick brown fox jumps over the lazy dog 0123456789 abcdefghijklmnopqrstuvwx'

export interface BenchResult {
  scenario: string
  ok: boolean
  error?: string
  metrics: Record<string, number | boolean | string>
}

type Scenario = (pane: BenchPane, options: BenchOptions) => Promise<Record<string, number | boolean | string>>

export interface BenchOptions {
  /** Multiplies the size of every workload. */
  scale?: number
}

let markerSeq = 0

function nextMarker(): string {
  return `__bench_done_${Date.now().toString(36)}_${++markerSeq}__`
}

// The echoed command must not itself contain the marker, or the wait ends the
// moment the shell echoes the line. Output is `<prefix><suffix>`; the command
// shows them apart.
function emitMarker(marker: string): string {
  const cut = Math.floor(marker.length / 2)
  return `printf '%s%s\\n' '${marker.slice(0, cut)}' '${marker.slice(cut)}'`
}

function nearTail(term: Terminal, marker: string): boolean {
  const buffer = term.buffer.active
  const bottom = buffer.baseY + buffer.cursorY
  for (let y = bottom; y >= Math.max(0, bottom - 4); y--) {
    if (buffer.getLine(y)?.translateToString(true).includes(marker)) return true
  }
  return false
}

function waitForMarker(term: Terminal, marker: string, timeoutMs = 180_000): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      handle.dispose()
      reject(new Error(`marker did not arrive within ${timeoutMs}ms`))
    }, timeoutMs)
    const handle = term.onWriteParsed(() => {
      if (!nearTail(term, marker)) return
      handle.dispose()
      clearTimeout(timer)
      resolve()
    })
  })
}

function nextFrame(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()))
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function settle(pane: BenchPane): Promise<void> {
  const marker = nextMarker()
  pane.send(`clear; ${emitMarker(marker)}\r`)
  await waitForMarker(pane.term, marker, 30_000)
  await sleep(300)
}

/** How late a 50ms timer runs is how long a click or a key would wait for the main thread. */
function watchLag(): () => { lagP50: number; lagP95: number; lagWorst: number } {
  const lags: number[] = []
  let expected = performance.now() + 50
  const timer = setInterval(() => {
    const now = performance.now()
    lags.push(Math.max(0, now - expected))
    expected = now + 50
  }, 50)
  return () => {
    clearInterval(timer)
    lags.sort((a, b) => a - b)
    return {
      lagP50: round(percentile(lags, 50)),
      lagP95: round(percentile(lags, 95)),
      lagWorst: round(lags[lags.length - 1] ?? 0),
    }
  }
}

function watchBacklog(pane: BenchPane): () => number {
  let max = 0
  const timer = setInterval(() => {
    max = Math.max(max, pane.backlog())
  }, 16)
  return () => {
    clearInterval(timer)
    return max
  }
}

interface KeySample {
  total: number
  parsed: number
  render: number
}

async function timeKeys(pane: BenchPane, count: number): Promise<KeySample[]> {
  const { term } = pane
  const samples: KeySample[] = []
  for (let i = 0; i < count; i++) {
    const at = performance.now()
    const char = String.fromCharCode(97 + (i % 26))
    let parsedAt = 0
    const echoed = new Promise<void>((resolve) => {
      const parsedHandle = term.onWriteParsed(() => {
        if (!nearTailChar(term, char)) return
        parsedHandle.dispose()
        parsedAt = performance.now()
        const renderHandle = term.onRender(() => {
          renderHandle.dispose()
          resolve()
        })
      })
    })
    term.input(char, true)
    await echoed
    const done = performance.now()
    samples.push({ total: done - at, parsed: parsedAt - at, render: done - parsedAt })
    await sleep(25 + (i % 4) * 10)
  }
  return samples
}

function nearTailChar(term: Terminal, char: string): boolean {
  const buffer = term.buffer.active
  const line = buffer.getLine(buffer.baseY + buffer.cursorY)?.translateToString(true) ?? ''
  return line.endsWith(char)
}

function keyStats(samples: KeySample[]): Record<string, number> {
  const sorted = (pick: (sample: KeySample) => number): number[] => samples.map(pick).sort((a, b) => a - b)
  const total = sorted((sample) => sample.total)
  const parsed = sorted((sample) => sample.parsed)
  const render = sorted((sample) => sample.render)
  return {
    echoP50: round(percentile(total, 50)),
    echoP95: round(percentile(total, 95)),
    echoWorst: round(total[total.length - 1] ?? 0),
    toParsedP50: round(percentile(parsed, 50)),
    toParsedP95: round(percentile(parsed, 95)),
    parsedToRenderP50: round(percentile(render, 50)),
    parsedToRenderP95: round(percentile(render, 95)),
  }
}

async function measureFlood(
  pane: BenchPane,
  command: string,
  bytes: number,
): Promise<Record<string, number | boolean | string> & FrameSummary> {
  await settle(pane)
  const marker = nextMarker()
  const stopFrames = collectFrames()
  const stopLag = watchLag()
  const stopBacklog = watchBacklog(pane)
  const startedAt = performance.now()
  pane.send(`${command}; ${emitMarker(marker)}\r`)
  await waitForMarker(pane.term, marker)
  await nextFrame()
  const elapsedMs = performance.now() - startedAt
  return {
    elapsedMs: round(elapsedMs),
    mbPerSec: round(bytes / 1e6 / (elapsedMs / 1000)),
    maxBacklogBytes: stopBacklog(),
    ...stopLag(),
    ...stopFrames(),
  }
}

// awk rather than a checked-in script: every rig has it, and the producer has
// to outrun the terminal for the terminal to be what is being measured.
const scenarios: Record<string, Scenario> = {
  // A plain wall of text, the `cat large.log` case.
  wall(pane, { scale = 1 }) {
    const lines = Math.round(200_000 * scale)
    const command = `awk 'BEGIN{for(i=0;i<${lines};i++)print i " ${LINE_TEXT}"}'`
    return measureFlood(pane, command, lines * (LINE_TEXT.length + 7))
  },

  // The same wall in color: every line changes SGR state several times, which is
  // the parser's slow path and what `git log -p` or a test runner produces.
  wallColor(pane, { scale = 1 }) {
    const lines = Math.round(120_000 * scale)
    const command = `awk 'BEGIN{for(i=0;i<${lines};i++)printf "\\033[3%dm%07d\\033[0m \\033[1;3%dmthe quick\\033[0m brown \\033[4mfox\\033[0m jumps over the lazy dog \\033[38;5;%dm0123456789\\033[0m\\n",i%8,i,(i+3)%8,i%256}'`
    return measureFlood(pane, command, lines * 112)
  },

  // Full-screen repaints inside synchronized-output markers, as an agent TUI
  // draws them, as fast as the producer can go.
  tui(pane, { scale = 1 }) {
    const repaints = Math.round(1500 * scale)
    const { rows, cols } = pane.term
    const row = 'x'.repeat(Math.max(1, cols - 1))
    const command = `awk 'BEGIN{for(f=0;f<${repaints};f++){printf "\\033[?2026h\\033[H";for(r=1;r<${rows};r++)printf "\\033[3%dm%s\\033[K\\r\\n",(f+r)%8,"${row}";printf "\\033[0mrepaint %d\\033[K\\033[?2026l",f}}'`
    return measureFlood(pane, command, repaints * rows * cols)
  },

  // Keystroke to echo at a person's pace on a quiet pane, split at the socket,
  // the parser and the renderer. `cat` holds the tty so the shell's own line
  // editor is not what is being timed.
  async typing(pane, { scale = 1 }) {
    await settle(pane)
    pane.send('cat\r')
    await sleep(300)
    const samples = await timeKeys(pane, Math.round(120 * scale))
    pane.send('\x03')
    await sleep(200)
    return { keys: samples.length, ...keyStats(samples) }
  },

  // Ctrl+C on a pane spewing output: from the key to a prompt that answers. The
  // gap is everything queued between tmux and the screen, so this is what makes
  // a runaway `cat` feel stuck.
  async interrupt(pane, { scale = 1 }) {
    await settle(pane)
    pane.send(`yes '${LINE_TEXT}'\r`)
    await sleep(3000 * scale)
    const backlogAtKey = pane.backlog()
    const marker = nextMarker()
    const startedAt = performance.now()
    pane.send('\x03')
    pane.send(`${emitMarker(marker)}\r`)
    await waitForMarker(pane.term, marker, 120_000)
    await nextFrame()
    return { interruptMs: round(performance.now() - startedAt), backlogAtKey }
  },

  // Wheel through scrollback at one notch per frame, both ways.
  async scroll(pane, { scale = 1 }) {
    await settle(pane)
    const marker = nextMarker()
    pane.send(`seq 1 4000; ${emitMarker(marker)}\r`)
    await waitForMarker(pane.term, marker)
    await sleep(500)
    const target = pane.term.element?.querySelector('.xterm-screen') ?? pane.term.element
    if (!target) throw new Error('the pane has no element to scroll')
    const stopFrames = collectFrames()
    const stopLag = watchLag()
    const durationMs = 4000 * scale
    const startedAt = performance.now()
    let events = 0
    let moved = 0
    let last = pane.term.buffer.active.viewportY
    const rendered: number[] = []
    const renderHandle = pane.term.onRender(() => {
      rendered.push(performance.now())
    })
    while (performance.now() - startedAt < durationMs) {
      const elapsed = performance.now() - startedAt
      const up = Math.floor(elapsed / 1000) % 2 === 0
      target.dispatchEvent(
        new WheelEvent('wheel', { deltaY: up ? -120 : 120, deltaMode: 0, bubbles: true, cancelable: true }),
      )
      events++
      await nextFrame()
      const y = pane.term.buffer.active.viewportY
      moved += Math.abs(y - last)
      last = y
    }
    renderHandle.dispose()
    pane.term.scrollToBottom()
    return { events, linesMoved: moved, renders: rendered.length, ...stopLag(), ...stopFrames() }
  },

  // Keystrokes in the visible pane while a pane nobody can see floods the same
  // socket. Hidden panes keep parsing, so this is the contention a background
  // agent puts on the one being typed into.
  async typingUnderHiddenFlood(pane, { scale = 1 }) {
    await settle(pane)
    const lines = Math.round(1_200_000 * scale)
    const flood = `awk "BEGIN{for(i=0;i<${lines};i++)print i, \\"${LINE_TEXT}\\"}"`
    pane.send(`tmux new-window -d -n bench-flood '${flood}'; cat\r`)
    await sleep(600)
    const stopFrames = collectFrames()
    const samples = await timeKeys(pane, Math.round(60 * scale))
    const frames = stopFrames()
    pane.send('\x03')
    await sleep(200)
    pane.send('tmux kill-window -t bench-flood 2>/dev/null\r')
    await sleep(500)
    return { keys: samples.length, ...keyStats(samples), ...frames }
  },

  // A full re-attach: the control client's first paint of the screen and its
  // scrollback, through the socket, into a new emulator.
  async attach(pane, { scale = 1 }) {
    await settle(pane)
    const marker = nextMarker()
    pane.send(`seq 1 ${Math.round(50_000 * scale)}; ${emitMarker(marker)}\r`)
    await waitForMarker(pane.term, marker)
    await sleep(500)
    const stopFrames = collectFrames()
    const startedAt = performance.now()
    await pane.reattach()
    await nextFrame()
    return { attachMs: round(performance.now() - startedAt), ...stopFrames() }
  },

  // Tab to tab and back, on a session with a window of history in each.
  async switchWindow(pane, { scale = 1 }) {
    await settle(pane)
    const marker = nextMarker()
    pane.send(`tmux new-window -d -n bench-tab 'seq 1 2000; exec sh'; ${emitMarker(marker)}\r`)
    await waitForMarker(pane.term, marker)
    await sleep(800)
    const stopFrames = collectFrames()
    const times: number[] = []
    for (let i = 0; i < Math.round(20 * scale); i++) {
      const startedAt = performance.now()
      await pane.nextWindow()
      await nextFrame()
      await nextFrame()
      times.push(performance.now() - startedAt)
      await sleep(120)
    }
    const frames = stopFrames()
    times.sort((a, b) => a - b)
    pane.send('tmux kill-window -t bench-tab 2>/dev/null\r')
    await sleep(400)
    return { switchP50: round(percentile(times, 50)), switchP95: round(percentile(times, 95)), ...frames }
  },
}

let lastRun: Promise<BenchResult[]> | null = null
let lastResults: BenchResult[] | null = null

async function run(names: string[], options: BenchOptions): Promise<BenchResult[]> {
  const results: BenchResult[] = []
  for (const scenario of names) {
    const pane = visibleBenchPane()
    const work = scenarios[scenario]
    if (!pane) {
      results.push({ scenario, ok: false, error: 'no terminal pane is on screen', metrics: {} })
      continue
    }
    if (!work) {
      results.push({ scenario, ok: false, error: 'unknown scenario', metrics: {} })
      continue
    }
    const startedAt = performance.now()
    // A covered window fires no animation frames, so timings from one measure
    // the OS. They are kept, and flagged, because a rerun costs minutes.
    let hiddenDuringRun = document.hidden
    const noteHidden = (): void => {
      hiddenDuringRun ||= document.hidden
    }
    document.addEventListener('visibilitychange', noteHidden)
    try {
      const metrics = await work(pane, options)
      metrics.hiddenDuringRun = hiddenDuringRun
      metrics.webgl = pane.webgl()
      metrics.cols = pane.term.cols
      metrics.rows = pane.term.rows
      perf.record('scenario', performance.now() - startedAt, { scenario, ...metrics })
      results.push({ scenario, ok: true, metrics })
    } catch (error) {
      results.push({ scenario, ok: false, error: error instanceof Error ? error.message : String(error), metrics: {} })
    } finally {
      document.removeEventListener('visibilitychange', noteHidden)
    }
  }
  await flushPerf()
  return results
}

export interface TerminalBench {
  scenarios: string[]
  /** Runs the named scenarios, every one when none are named. */
  start(names?: string[], options?: BenchOptions): void
  /** Null while a run is in flight; a native js_eval call cannot wait out a long one. */
  poll(): BenchResult[] | null
}

export const terminalBench: TerminalBench = {
  scenarios: Object.keys(scenarios),
  start(names = Object.keys(scenarios), options = {}) {
    lastResults = null
    lastRun = run(names, options).then((results) => {
      lastResults = results
      return results
    })
    void lastRun
  },
  poll: () => lastResults,
}
