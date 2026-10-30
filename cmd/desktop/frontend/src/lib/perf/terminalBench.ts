import { nextTick } from 'vue'
import type { IDisposable, Terminal } from '@xterm/xterm'
import { flushPerf, usePerf } from '../../composables/usePerf'
import { collectFrames, percentile, round, type FrameSummary } from '../frameCollector'
import { benchPanes, visibleBenchPane, type BenchPane } from '../terminalPerf'

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
  /** The program switchRealTui runs behind its second tab, instead of top. */
  tuiCommand?: string
  /** Overrides the pane's smoothScrollDuration for the run, to compare settings. */
  smoothScrollMs?: number
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

interface AtlasRenderer {
  _canvas?: HTMLCanvasElement
  _gl?: WebGL2RenderingContext
}

// The renderer's own canvas: a pane holds several, and asking the first one for a
// webgl2 context either answers null on a 2D canvas or claims a blank one.
function atlasRenderer(term: Terminal): AtlasRenderer | undefined {
  const core = term as unknown as { _core?: { _renderService?: { _renderer?: { value?: AtlasRenderer } } } }
  return core._core?._renderService?._renderer?.value
}

// Read straight after a draw, before the browser composites and may clear the
// buffer: a canvas of one color holds no text, which on screen is a blank flash.
function canvasIsBlank(term: Terminal): boolean {
  const { _canvas: canvas, _gl: gl } = atlasRenderer(term) ?? {}
  if (!canvas || !gl) return false
  const width = Math.min(canvas.width, 400)
  const height = Math.min(canvas.height, 300)
  const pixels = new Uint8Array(width * height * 4)
  gl.readPixels(0, canvas.height - height, width, height, gl.RGBA, gl.UNSIGNED_BYTE, pixels)
  for (let i = 4; i < pixels.length; i += 4) {
    if (pixels[i] !== pixels[0] || pixels[i + 1] !== pixels[1] || pixels[i + 2] !== pixels[2]) return false
  }
  return true
}

let capturedReveal: string | null = null

// A data URL of the canvas as of the last draw, which is how a run shows real
// pixels: the page cannot screenshot itself and the window may refuse the OS.
function captureCanvas(term: Terminal): string | null {
  return atlasRenderer(term)?._canvas?.toDataURL('image/png') ?? null
}

// The buffer is only defined right after a draw, so this asks for one and reads it
// in the same callback.
function captureCanvasAtNextDraw(term: Terminal): string | null {
  let captured: string | null = null
  const handle = term.onRender(() => {
    captured = captureCanvas(term)
    handle.dispose()
  })
  const service = (
    term as unknown as {
      _core?: { _renderService?: { _renderRows?: (s: number, e: number) => void; _isNextRenderRedrawOnly?: boolean } }
    }
  )._core?._renderService
  if (service?._renderRows) {
    service._isNextRenderRedrawOnly = false
    service._renderRows(0, term.rows - 1)
  }
  handle.dispose()
  return captured
}

// A draw on demand, then the check: the buffer is only defined right after one.
function blankAfterDraw(term: Terminal): boolean {
  let blank = false
  const handle = term.onRender(() => {
    blank = canvasIsBlank(term)
  })
  const service = (
    term as unknown as {
      _core?: { _renderService?: { _renderRows?: (s: number, e: number) => void; _isNextRenderRedrawOnly?: boolean } }
    }
  )._core?._renderService
  if (service?._renderRows) {
    service._isNextRenderRedrawOnly = false
    service._renderRows(0, term.rows - 1)
  }
  handle.dispose()
  return blank
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

async function fillScrollback(pane: BenchPane): Promise<void> {
  await settle(pane)
  const marker = nextMarker()
  pane.send(`seq 1 4000; ${emitMarker(marker)}\r`)
  await waitForMarker(pane.term, marker)
  await sleep(500)
}

async function driveWheel(
  pane: BenchPane,
  deltaY: number,
  durationMs: number,
): Promise<Record<string, number | boolean | string>> {
  const target = pane.term.element?.querySelector('.xterm-screen') ?? pane.term.element
  if (!target) throw new Error('the pane has no element to scroll')
  const stopFrames = collectFrames()
  const stopLag = watchLag()
  const startedAt = performance.now()
  let events = 0
  let moved = 0
  let renders = 0
  let last = pane.term.buffer.active.viewportY
  let furthest = 0
  const renderHandle = pane.term.onRender(() => {
    renders++
  })
  while (performance.now() - startedAt < durationMs) {
    const up = Math.floor((performance.now() - startedAt) / 1000) % 2 === 0
    target.dispatchEvent(new WheelEvent('wheel', { deltaY: up ? -deltaY : deltaY, bubbles: true, cancelable: true }))
    events++
    await nextFrame()
    const y = pane.term.buffer.active.viewportY
    moved += Math.abs(y - last)
    last = y
    furthest = Math.max(furthest, pane.term.buffer.active.baseY - y)
  }
  renderHandle.dispose()
  pane.term.scrollToBottom()
  return { events, linesMoved: moved, furthestFromTail: furthest, renders, ...stopLag(), ...stopFrames() }
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

  // A wheel through scrollback at one notch per frame, both ways.
  async scroll(pane, { scale = 1 }) {
    await fillScrollback(pane)
    return driveWheel(pane, 120, 4000 * scale)
  },

  // Small deltas at frame rate, the way a trackpad reports a two-finger drag.
  async scrollTrackpad(pane, { scale = 1 }) {
    await fillScrollback(pane)
    return driveWheel(pane, 12, 4000 * scale)
  },

  // Reading history while the pane keeps printing at an agent's pace. The viewport
  // has to follow the wheel under the parser. A flood is no test of this: it trims
  // the whole scrollback in about 100ms, which pushes any viewport to the top.
  async scrollUnderOutput(pane, { scale = 1 }) {
    await fillScrollback(pane)
    pane.send('while :; do seq 1 20; sleep 0.05; done\r')
    await sleep(200)
    const metrics = await driveWheel(pane, 120, 3000 * scale)
    pane.send('\x03')
    await sleep(300)
    return metrics
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

  // Sidebar clicks between two real sessions: the first visit attaches and the
  // rest flip a pooled pane, each with a TUI redrawing behind it. Needs two
  // started sessions besides the scratch terminal.
  async switchSession(_pane, { scale = 1 }) {
    const rows = [...document.querySelectorAll<HTMLElement>('[data-testid="terminal-session-row"][data-slug]')].filter(
      (row) => row.dataset.slug !== 'Scratch' && row.offsetParent,
    )
    if (rows.length < 2) throw new Error(`need two sessions in the sidebar, found ${rows.length}`)
    const [first, second] = rows
    const loop = "while :; do printf '\\033[H\\033[2J%s\\n' $(date +%s); seq 1 30; sleep 0.05; done"

    const cold: number[] = []
    let coldBlank = 0
    for (const row of [first, second]) {
      const revealed = await clickAndWaitForReveal(row, visibleBenchPane())
      cold.push(revealed.ms)
      if (revealed.blank) coldBlank++
      await sleep(600)
      revealed.pane?.send(`${loop}\r`)
      await sleep(500)
    }

    const stopFrames = collectFrames()
    const warm: number[] = []
    const warmFresh: number[] = []
    let warmBlank = 0
    for (let i = 0; i < Math.round(30 * scale); i++) {
      const row = i % 2 === 0 ? first : second
      const revealed = await clickAndWaitForReveal(row, visibleBenchPane())
      warm.push(revealed.ms)
      if (revealed.freshFrames >= 0) warmFresh.push(revealed.freshFrames)
      if (revealed.blank) warmBlank++
      await sleep(150)
    }
    const frames = stopFrames()
    warm.sort((a, b) => a - b)
    warmFresh.sort((a, b) => a - b)
    for (const row of [first, second]) {
      await clickAndWaitForReveal(row, visibleBenchPane())
      visibleBenchPane()?.send('\x03')
      await sleep(150)
    }
    return {
      coldMs: round(Math.max(...cold)),
      coldBlank,
      warmP50: round(percentile(warm, 50)),
      warmP95: round(percentile(warm, 95)),
      warmBlank,
      warmFreshFrameP50: percentile(warmFresh, 50),
      warmFreshFrameP95: percentile(warmFresh, 95),
      ...frames,
    }
  },

  // Real full-screen programs instead of a redraw loop. `redrawBytes` is what the
  // revealed program sent in the 300ms after a switch: more than a screenful means
  // the switch itself made it repaint, by a resize or a focus report, which no
  // renderer fix can hide.
  async switchRealTui(pane, { scale = 1, tuiCommand = 'top' }) {
    await settle(pane)
    const marker = nextMarker()
    pane.send(`tmux new-window -d -n bench-top "${tuiCommand}"; ${emitMarker(marker)}\r`)
    await waitForMarker(pane.term, marker)
    await sleep(300)
    pane.send("vim -u NONE -c 'set mouse=a' -c 'help' -c 'only'\r")
    await sleep(1200)

    const redraw: number[] = []
    const times: number[] = []
    let blank = 0
    let resized = 0
    capturedReveal = null
    for (let i = 0; i < Math.round(20 * scale); i++) {
      const outgoing = visibleBenchPane()
      const sizes = new Map(benchPanes().map((candidate) => [candidate, candidate.term.cols * candidate.term.rows]))
      const before = new Map(benchPanes().map((candidate) => [candidate, candidate.received()]))
      const startedAt = performance.now()
      await outgoing?.nextWindow()
      await nextFrame()
      times.push(performance.now() - startedAt)
      const shown = visibleBenchPane()
      if (!shown) continue
      await sleep(300)
      if (blankAfterDraw(shown.term)) blank++
      capturedReveal = captureCanvasAtNextDraw(shown.term) ?? capturedReveal
      redraw.push(shown.received() - (before.get(shown) ?? 0))
      if (sizes.get(shown) !== shown.term.cols * shown.term.rows) resized++
    }
    for (const samples of [redraw, times]) samples.sort((a, b) => a - b)
    const vimPane = benchPanes().find((candidate) => candidate.term.buffer.active.type === 'alternate')
    const shown = visibleBenchPane()
    if (shown && shown !== pane) shown.send('q')
    vimPane?.send('\x1b:qa!\r')
    await sleep(400)
    pane.send('tmux kill-window -t bench-top 2>/dev/null; clear\r')
    await sleep(300)
    return {
      switchP50: round(percentile(times, 50)),
      switchP95: round(percentile(times, 95)),
      redrawBytesP50: percentile(redraw, 50),
      redrawBytesP95: percentile(redraw, 95),
      screenBytes: pane.term.cols * pane.term.rows,
      resized,
      blank,
    }
  },

  // A real width change, the way toggling the sidebar makes one: from the click to
  // the pane at its new grid, repainted. A TUI inside redraws on the way.
  async resize(pane, { scale = 1 }) {
    await settle(pane)
    const toggle = document.querySelector<HTMLElement>('[data-testid="titlebar-toggle-sidebar"]')
    if (!toggle) throw new Error('no sidebar toggle to resize with')
    const times: number[] = []
    for (let i = 0; i < Math.round(10 * scale) * 2; i++) {
      const cols = pane.term.cols
      const startedAt = performance.now()
      toggle.click()
      const settled = new Promise<boolean>((resolve) => {
        const handle = pane.term.onResize(() => {
          handle.dispose()
          resolve(true)
        })
        setTimeout(() => {
          handle.dispose()
          resolve(false)
        }, 2000)
      })
      const changed = await settled
      await nextFrame()
      if (changed && pane.term.cols !== cols) times.push(performance.now() - startedAt)
      await sleep(250)
    }
    times.sort((a, b) => a - b)
    return {
      resizeSamples: times.length,
      resizeP50: round(percentile(times, 50)),
      resizeP95: round(percentile(times, 95)),
    }
  },

  // Tab to tab between two windows that are both redrawing, as agent TUIs do, so
  // the screen a hidden pane last drew is out of date by the time it comes back.
  // `freshFrameP50` counts the frames shown before the pane's current contents:
  // 0 means the first frame after the switch was right, and each one above that
  // is a frame of old screen, which is the flash.
  async switchWindow(pane, { scale = 1 }) {
    await settle(pane)
    const loop = "while :; do printf '\\033[H\\033[2J%s\\n' $(date +%s); seq 1 30; sleep 0.05; done"
    const marker = nextMarker()
    pane.send(`tmux new-window -d -n bench-tab "${loop}"; ${emitMarker(marker)}\r`)
    await waitForMarker(pane.term, marker)
    pane.send(`${loop}\r`)
    await sleep(800)

    const stopFrames = collectFrames()
    const times: number[] = []
    const flips: number[] = []
    const freshFrames: number[] = []
    let blankReveals = 0
    for (let i = 0; i < Math.round(30 * scale); i++) {
      const outgoing = visibleBenchPane()
      const startedAt = performance.now()
      let frames = 0
      let counting = true
      const count = (): void => {
        frames++
        if (counting) requestAnimationFrame(count)
      }
      requestAnimationFrame(count)
      const rendered = new Promise<number>((resolve) => {
        const handles = benchPanes().map((candidate) =>
          candidate.term.onRender(() => {
            if (candidate === outgoing) return
            for (const handle of handles) handle.dispose()
            if (canvasIsBlank(candidate.term)) blankReveals++
            resolve(frames)
          }),
        )
      })
      // The tmux round trip behind the select is not what the eye waits for, so
      // the flip and the frame it lands on are timed apart from it.
      const selected = outgoing?.nextWindow()
      await nextTick()
      flips.push(performance.now() - startedAt)
      await nextFrame()
      times.push(performance.now() - startedAt)
      await selected
      const fresh = await Promise.race([rendered, sleep(500).then(() => -1)])
      counting = false
      if (fresh >= 0) freshFrames.push(fresh)
      await sleep(150)
    }
    const frames = stopFrames()
    times.sort((a, b) => a - b)
    flips.sort((a, b) => a - b)
    freshFrames.sort((a, b) => a - b)
    pane.send('\x03')
    const current = visibleBenchPane() ?? pane
    current.send('\x03')
    await sleep(100)
    current.send('tmux kill-window -t bench-tab 2>/dev/null; clear\r')
    await sleep(400)
    return {
      flipP50: round(percentile(flips, 50)),
      flipP95: round(percentile(flips, 95)),
      switchP50: round(percentile(times, 50)),
      switchP95: round(percentile(times, 95)),
      freshFrameP50: percentile(freshFrames, 50),
      freshFrameP95: percentile(freshFrames, 95),
      freshSamples: freshFrames.length,
      blankReveals,
      ...frames,
    }
  },
}

// The first frame the incoming pane is on screen is the one that matters: a
// click's feel is the time to it, and its contents are the flash or the lack of one.
async function clickAndWaitForReveal(
  row: HTMLElement,
  outgoing: BenchPane | undefined,
): Promise<{ ms: number; blank: boolean; pane: BenchPane | undefined; freshFrames: number }> {
  const startedAt = performance.now()
  let frame = 0
  let renderedAt = -1
  // Only a draw makes the buffer readable, and a reveal with nothing new to draw
  // keeps the screen it composited last, so no draw is not a blank.
  let blank = false
  const watching = new Map<BenchPane, IDisposable>()
  const watchPanes = (): void => {
    for (const candidate of benchPanes()) {
      if (candidate === outgoing || watching.has(candidate)) continue
      watching.set(
        candidate,
        candidate.term.onRender(() => {
          if (renderedAt >= 0 || !candidate.term.element?.offsetParent) return
          renderedAt = frame
          blank = canvasIsBlank(candidate.term)
        }),
      )
    }
  }
  watchPanes()
  row.click()
  try {
    for (;;) {
      await nextFrame()
      watchPanes()
      const shown = visibleBenchPane()
      if (shown && shown !== outgoing) {
        const ms = performance.now() - startedAt
        const revealedAt = frame
        while (renderedAt < 0 && frame - revealedAt < 30) {
          await nextFrame()
          frame++
        }
        return { ms, blank, pane: shown, freshFrames: renderedAt < 0 ? -1 : Math.max(0, renderedAt - revealedAt) }
      }
      frame++
      if (performance.now() - startedAt > 5000) return { ms: 5000, blank: true, pane: shown, freshFrames: -1 }
    }
  } finally {
    for (const handle of watching.values()) handle.dispose()
  }
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
    const smoothScroll = pane.term.options.smoothScrollDuration
    if (options.smoothScrollMs !== undefined) pane.term.options.smoothScrollDuration = options.smoothScrollMs
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
      pane.term.options.smoothScrollDuration = smoothScroll
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
  /** What the first reveal of the last `switchRealTui` drew, as a data URL. */
  lastReveal(): string | null
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
  lastReveal: () => capturedReveal,
}
