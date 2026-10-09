import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import type { ISearchOptions } from '@xterm/addon-search'
import { resetTerminalFacesForTests, useTerminalWindows } from '../useTerminalWindows'
import {
  defaultTerminalFontSizePx,
  defaultTerminalFontWeight,
  defaultTerminalFontWeightBold,
  defaultTerminalLetterSpacing,
  defaultTerminalLineHeight,
  useTerminalFont,
} from '../../stores/useTerminalFont'
import { SYMBOL_FONT, TERMINAL_FONT, terminalFontStack } from '../../lib/terminalFaces'
import { TerminalRequestError, type PaneLayout, type TerminalClient } from '../../lib/terminalClient'
import { paneMayAutoFocus } from '../../lib/terminalTree'
import { useKeybindings } from '../useKeybindings'
import { useTerminalOutcomes } from '../../stores/useTerminalOutcomes'

const xterm = vi.hoisted(() => {
  interface FakeLine {
    isWrapped: boolean
    getCell: (column: number) => { getChars: () => string; getWidth: () => number } | undefined
  }

  class FakeTerminal {
    cols = 80
    rows = 24
    options: Record<string, unknown> = {}
    write = vi.fn()
    focus = vi.fn()
    element?: HTMLElement
    open = vi.fn((host: HTMLElement) => {
      this.element = host
    })
    loadAddon = vi.fn((addon: { activate?: (term: FakeTerminal) => void }) => addon.activate?.(this))
    dispose = vi.fn()
    resizeEffect?: (term: FakeTerminal) => void
    resize = vi.fn((cols: number, rows: number) => {
      this.cols = cols
      this.rows = rows
      this.resizeEffect?.(this)
    })
    onDataDisposed = false
    input = vi.fn((data: string) => this.type(data))
    _core: { _renderService?: { dimensions: { css: { cell: { width: number; height: number } } } } } = {
      _renderService: { dimensions: { css: { cell: { width: 10, height: 25 } } } },
    }
    parser = {
      registerCsiHandler: vi.fn(() => ({ dispose: vi.fn() })),
      registerDcsHandler: vi.fn(() => ({ dispose: vi.fn() })),
      registerOscHandler: vi.fn(() => ({ dispose: vi.fn() })),
    }
    private lines = new Map<number, FakeLine>()
    buffer = {
      active: {
        type: 'normal' as 'normal' | 'alternate',
        cursorY: 0,
        viewportY: 0,
        baseY: 0,
        length: 0,
        getLine: (line: number) => this.lines.get(line),
      },
      onBufferChange: (handler: () => void) => {
        this.bufferHandlers.push(handler)
        return { dispose: () => {} }
      },
    }
    markers: { line: number; dispose: ReturnType<typeof vi.fn> }[] = []
    registerMarker = vi.fn((cursorYOffset = 0) => {
      const marker = {
        line: this.buffer.active.baseY + this.buffer.active.cursorY + cursorYOffset,
        dispose: vi.fn(() => {
          marker.line = -1
        }),
      }
      this.markers.push(marker)
      return marker
    })
    scrollToLine = vi.fn((line: number) => {
      this.buffer.active.viewportY = Math.min(line, this.buffer.active.baseY)
    })
    scrollToBottom = vi.fn(() => {
      this.buffer.active.viewportY = this.buffer.active.baseY
      for (const handler of this.scrollHandlers) handler()
    })
    reset = vi.fn(() => {
      this.buffer.active.baseY = 0
      this.buffer.active.viewportY = 0
      this.lines.clear()
    })
    private handlers: ((data: string) => void)[] = []
    private scrollHandlers: (() => void)[] = []
    private bufferHandlers: (() => void)[] = []
    private keyHandler?: (event: KeyboardEvent) => boolean

    constructor(options: Record<string, unknown> = {}) {
      this.options = { ...options }
      FakeTerminal.instances.push(this)
    }

    onData(handler: (data: string) => void) {
      this.handlers.push(handler)
      return {
        dispose: () => {
          this.onDataDisposed = true
        },
      }
    }

    onScroll(handler: () => void) {
      this.scrollHandlers.push(handler)
      return { dispose: () => {} }
    }

    attachCustomKeyEventHandler(handler: (event: KeyboardEvent) => boolean) {
      this.keyHandler = handler
    }

    press(event: Partial<KeyboardEvent>): boolean {
      return this.keyHandler?.({ type: 'keydown', preventDefault: () => {}, ...event } as KeyboardEvent) ?? true
    }

    type(data: string): void {
      for (const handler of this.handlers) handler(data)
    }

    scrollTo(viewportY: number, baseY: number): void {
      this.buffer.active.viewportY = viewportY
      this.buffer.active.baseY = baseY
      for (const handler of this.scrollHandlers) handler()
    }

    setLine(line: number, isWrapped: boolean): void {
      this.lines.set(line, {
        isWrapped,
        getCell: () => ({ getChars: () => 'x', getWidth: () => 1 }),
      })
    }

    clearLines(): void {
      this.lines.clear()
    }

    switchBuffer(): void {
      for (const handler of this.bufferHandlers) handler()
    }

    static instances: FakeTerminal[] = []
  }

  // Both renderer addons throw out of their constructor when the context they
  // need is missing, which is the fallback trigger the composable catches.
  class FakeWebglAddon {
    static instances: FakeWebglAddon[] = []
    static unavailable = false
    dispose = vi.fn()
    activate = vi.fn()
    private lossHandlers: (() => void)[] = []

    constructor() {
      if (FakeWebglAddon.unavailable) throw new Error('WebGL2 not supported')
      FakeWebglAddon.instances.push(this)
    }

    onContextLoss(handler: () => void) {
      this.lossHandlers.push(handler)
      return { dispose: () => {} }
    }

    loseContext(): void {
      for (const handler of this.lossHandlers) handler()
    }
  }

  class FakeCanvasAddon {
    static instances: FakeCanvasAddon[] = []
    static unavailable = false
    dispose = vi.fn()
    activate = vi.fn()

    constructor() {
      if (FakeCanvasAddon.unavailable) throw new Error('no 2d context')
      FakeCanvasAddon.instances.push(this)
    }
  }

  class FakeWebLinksAddon {
    static instances: FakeWebLinksAddon[] = []
    dispose = vi.fn()
    activate = vi.fn()

    constructor(readonly handler: (event: MouseEvent, uri: string) => void) {
      FakeWebLinksAddon.instances.push(this)
    }
  }

  // The real addon reports hit counts through onDidChangeResults, so the fake
  // has to be driven the same way: a find only produces a count if it fires.
  // It also reproduces the one way a find fails outright — highlighting goes
  // through Terminal.registerDecoration, which is proposed API and throws on a
  // terminal that did not opt in, so a search that highlights is dead without
  // it and no assertion about the results would ever be reached.
  class FakeSearchAddon {
    static instances: FakeSearchAddon[] = []
    static results: { resultIndex: number; resultCount: number } = { resultIndex: 0, resultCount: 1 }
    dispose = vi.fn()
    activate = vi.fn((term: FakeTerminal) => {
      this.proposedApi = term.options.allowProposedApi === true
    })
    clearDecorations = vi.fn()
    findNext = vi.fn((term: string, options?: ISearchOptions) => this.record('next', term, options))
    findPrevious = vi.fn((term: string, options?: ISearchOptions) => this.record('previous', term, options))
    calls: { mode: string; term: string; options?: ISearchOptions }[] = []
    private proposedApi = false
    private resultHandlers: ((results: { resultIndex: number; resultCount: number }) => void)[] = []

    constructor() {
      FakeSearchAddon.instances.push(this)
    }

    onDidChangeResults(handler: (results: { resultIndex: number; resultCount: number }) => void) {
      this.resultHandlers.push(handler)
      return { dispose: () => {} }
    }

    private record(mode: string, term: string, options?: ISearchOptions): boolean {
      if (options?.decorations && !this.proposedApi) {
        throw new Error('You must set the allowProposedApi option to true to use proposed API')
      }
      this.calls.push({ mode, term, options })
      for (const handler of this.resultHandlers) handler(FakeSearchAddon.results)
      return true
    }
  }

  return { FakeTerminal, FakeWebglAddon, FakeCanvasAddon, FakeSearchAddon, FakeWebLinksAddon }
})

const wails = vi.hoisted(() => ({ OpenURL: vi.fn(() => Promise.resolve()) }))

vi.mock('@xterm/xterm', () => ({ Terminal: xterm.FakeTerminal }))
vi.mock('@xterm/addon-search', () => ({ SearchAddon: xterm.FakeSearchAddon }))
vi.mock('@xterm/addon-webgl', () => ({ WebglAddon: xterm.FakeWebglAddon }))
vi.mock('@xterm/addon-canvas', () => ({ CanvasAddon: xterm.FakeCanvasAddon }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: xterm.FakeWebLinksAddon }))
vi.mock('@wailsio/runtime', () => ({ Browser: { OpenURL: wails.OpenURL }, Events: { On: vi.fn(() => vi.fn()) } }))
// The font store hydrates from settings.yaml on first use; an empty answer keeps
// every default, and the setters the tests call persist without a binding error.
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn(() => Promise.resolve({})),
  SetTerminalFontFamily: vi.fn(() => Promise.resolve()),
  SetTerminalFontSize: vi.fn(() => Promise.resolve()),
  SetTerminalFontWeights: vi.fn(() => Promise.resolve()),
  SetTerminalLetterSpacing: vi.fn(() => Promise.resolve()),
  SetTerminalLineHeight: vi.fn(() => Promise.resolve()),
  SetKeybindingSettings: vi.fn(() => Promise.resolve()),
}))

const encoder = new TextEncoder()

class FakeSocket {
  static OPEN = 1
  readyState = 1
  binaryType = 'blob'
  sent: Uint8Array[] = []
  closed = false
  onopen: (() => void) | null = null
  onmessage: ((event: { data: unknown }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  send(frame: Uint8Array): void {
    this.sent.push(frame)
  }
  close(): void {
    this.closed = true
  }
}

class FakeResizeObserver {
  static instances: FakeResizeObserver[] = []
  observed: unknown[] = []
  disconnected = false

  constructor(private readonly callback: () => void) {
    FakeResizeObserver.instances.push(this)
  }

  observe(target: unknown): void {
    this.observed.push(target)
  }
  disconnect(): void {
    this.disconnected = true
  }
  trigger(): void {
    this.callback()
  }
}

let sockets: FakeSocket[] = []
let loadedFaces: string[] = []

function leaf(paneId: string, width: number, height: number, x = 0, y = 0): PaneLayout {
  return { paneId, x, y, width, height }
}

type MockedClient = { [K in keyof TerminalClient]: Mock<(...args: never[]) => unknown> }

function fakeClient(): MockedClient {
  return {
    attach: vi.fn().mockResolvedValue({
      windows: [
        {
          windowId: '@1',
          name: 'agent',
          active: true,
          activePane: '%1',
          width: 213,
          height: 55,
          zoomed: false,
          layout: leaf('%1', 213, 55),
        },
        {
          windowId: '@2',
          name: 'shell',
          active: false,
          activePane: '%2',
          width: 213,
          height: 55,
          zoomed: false,
          layout: leaf('%2', 213, 55),
        },
      ],
    }),
    start: vi.fn().mockResolvedValue({ started: true }),
    kill: vi.fn().mockResolvedValue({ killed: true }),
    listWindows: vi.fn().mockResolvedValue({}),
    resize: vi.fn().mockResolvedValue(undefined),
    newWindow: vi.fn().mockResolvedValue({ windowId: '@3' }),
    newAgentWindow: vi.fn().mockResolvedValue({ windowId: '@4' }),
    closeWindow: vi.fn().mockResolvedValue(undefined),
    windowForeground: vi.fn().mockResolvedValue({ running: false, command: '' }),
    renameWindow: vi.fn().mockResolvedValue(undefined),
    moveWindow: vi.fn().mockResolvedValue({ windows: [] }),
    selectWindow: vi.fn().mockResolvedValue(undefined),
    splitPane: vi.fn().mockResolvedValue({ paneId: '%9' }),
    selectPane: vi.fn().mockResolvedValue(undefined),
    closePane: vi.fn().mockResolvedValue(undefined),
    paneForeground: vi.fn().mockResolvedValue({ running: false, command: '' }),
    resizePane: vi.fn().mockResolvedValue(undefined),
    zoomPane: vi.fn().mockResolvedValue(undefined),
    detach: vi.fn().mockResolvedValue(undefined),
    openStream: vi.fn(() => {
      const socket = new FakeSocket()
      sockets.push(socket)
      return socket as unknown as WebSocket
    }),
  }
}

function open(client: MockedClient) {
  return useTerminalWindows('hive-abc', client as unknown as TerminalClient)
}

// happy-dom lays nothing out, so a plain div measures 0×0 — which the
// composable rightly refuses to vote from. A pane meant to be visible
// stubs its box.
// term.open() builds .xterm-viewport inside the host and the composable hangs
// its scroll listener off it. The fake Terminal opens nothing, so the host has
// to stand in for what xterm would have put there.
function paneHost(): HTMLElement {
  const host = document.createElement('div')
  const viewport = document.createElement('div')
  viewport.className = 'xterm-viewport'
  host.appendChild(viewport)
  resizeHost(host, 80, 24)
  return host
}

// The fake terminal's cell is 10×25, so a box is stated in the cells it fits.
function paneHostOf(cols: number, rows: number): HTMLElement {
  const host = paneHost()
  resizeHost(host, cols, rows)
  return host
}

function resizeHost(host: HTMLElement, cols: number, rows: number): void {
  Object.defineProperties(host, {
    clientWidth: { value: cols * 10, configurable: true },
    clientHeight: { value: rows * 25, configurable: true },
  })
}

// The fake terminal creates no pane DOM, so one host stands in for both the
// window box and each pane host.
function mountWindow(session: ReturnType<typeof open>, windowId: string, host: HTMLElement = paneHost()): HTMLElement {
  session.attachTab(windowId, host)
  const tab = session.tabs.value.find((candidate) => candidate.windowId === windowId)
  for (const pane of tab?.panes ?? []) session.attachPane(pane.paneId, host)
  return host
}

function outputFrame(windowId: string, paneId: string, data: string): ArrayBuffer {
  const win = encoder.encode(windowId)
  const pane = encoder.encode(paneId)
  const body = encoder.encode(data)
  const bytes = new Uint8Array(3 + win.length + pane.length + body.length)
  bytes[0] = 0x00
  bytes[1] = win.length
  bytes.set(win, 2)
  bytes[2 + win.length] = pane.length
  bytes.set(pane, 3 + win.length)
  bytes.set(body, 3 + win.length + pane.length)
  return bytes.buffer
}

function jsonFrame(kind: number, payload: unknown): ArrayBuffer {
  const body = encoder.encode(JSON.stringify(payload))
  const bytes = new Uint8Array(1 + body.length)
  bytes[0] = kind
  bytes.set(body, 1)
  return bytes.buffer
}

// Window events carry the whole window; the attach fixture's windows are 213x55,
// so that is what an event leaves a size at unless it says otherwise.
function windowFrame(
  kind: string,
  windowId: string,
  window: {
    name?: string
    active?: boolean
    activePane?: string
    width?: number
    height?: number
    zoomed?: boolean
    layout?: PaneLayout | null
  } = {},
): ArrayBuffer {
  const width = window.width ?? 213
  const height = window.height ?? 55
  const activePane = window.activePane ?? `%${windowId.slice(1)}`
  return jsonFrame(0x01, {
    kind,
    windowId,
    name: window.name ?? '',
    active: window.active ?? false,
    activePane,
    width,
    height,
    zoomed: window.zoomed ?? false,
    layout: window.layout === undefined ? leaf(activePane, width, height) : window.layout,
  })
}

async function attached(client: MockedClient = fakeClient()) {
  const session = open(client)
  await session.start()
  await flushPromises()
  sockets[0].onopen?.()
  return { client, session, socket: sockets[0] }
}

describe('useTerminalWindows', () => {
  beforeEach(() => {
    sockets = []
    loadedFaces = []
    paneMayAutoFocus.value = true
    // The keymap is a module singleton, so a rebind in one test would still be
    // in force in the next.
    useKeybindings().clearAll()
    resetTerminalFacesForTests()
    xterm.FakeTerminal.instances = []
    xterm.FakeWebglAddon.instances = []
    xterm.FakeCanvasAddon.instances = []
    xterm.FakeSearchAddon.instances = []
    xterm.FakeSearchAddon.results = { resultIndex: 0, resultCount: 1 }
    xterm.FakeWebglAddon.unavailable = false
    xterm.FakeCanvasAddon.unavailable = false
    xterm.FakeWebLinksAddon.instances = []
    wails.OpenURL.mockClear()
    FakeResizeObserver.instances = []
    // The remembered vote outlives a composable on purpose, so each test states
    // its own starting memory rather than inheriting the last one's.
    localStorage.clear()
    globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
    globalThis.ResizeObserver = FakeResizeObserver as unknown as typeof ResizeObserver
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: {
        load: vi.fn((font: string) => {
          loadedFaces.push(font)
          return Promise.resolve([])
        }),
      },
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    Reflect.deleteProperty(navigator, 'userAgent')
  })

  it('attaches, builds one terminal per window and goes live on the stream', async () => {
    const { session, socket } = await attached()

    expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@1', '@2'])
    expect(session.activeWindowId.value).toBe('@1')
    expect(xterm.FakeTerminal.instances).toHaveLength(2)
    expect(socket.binaryType).toBe('arraybuffer')
    expect(session.status.value).toBe('live')
  })

  it('writes an output frame to the terminal of its window', async () => {
    const { session, socket } = await attached()

    socket.onmessage?.({ data: outputFrame('@2', '%2', 'from the shell') })

    expect(session.tabs.value[0].panes[0].term.write).not.toHaveBeenCalled()
    const written = (session.tabs.value[1].panes[0].term.write as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(new TextDecoder().decode(written)).toBe('from the shell')
  })

  it('coalesces a synchronized redraw split across tmux output frames', async () => {
    const { client, session, socket } = await attached()
    const term = session.tabs.value[0].panes[0].term
    const write = term.write as ReturnType<typeof vi.fn>
    const resizeCalls = (term.resize as ReturnType<typeof vi.fn>).mock.calls.length

    socket.onmessage?.({ data: outputFrame('@1', '%1', '\x1b[?2026h\x1b[2Kworking') })
    expect(write).not.toHaveBeenCalled()

    socket.onmessage?.({ data: outputFrame('@1', '%1', '\r\n\x1b[2K> prompt\x1b[?2026l') })

    expect(write).toHaveBeenCalledTimes(1)
    expect(new TextDecoder().decode(write.mock.calls[0][0])).toBe(
      '\x1b[?2026h\x1b[2Kworking\r\n\x1b[2K> prompt\x1b[?2026l',
    )
    expect(term.resize).toHaveBeenCalledTimes(resizeCalls)
    expect(client.resize).not.toHaveBeenCalled()
  })

  // 'live' only says the socket opened; the first-paint capture is still in
  // flight then, and the session switcher must not reveal a blank grid.
  it('flags the first paint once a terminal has processed output', async () => {
    const { session, socket } = await attached()
    expect(session.painted.value).toBe(false)

    socket.onmessage?.({ data: outputFrame('@1', '%1', 'hello') })
    const write = session.tabs.value[0].panes[0].term.write as ReturnType<typeof vi.fn>
    expect(session.painted.value).toBe(false)
    write.mock.calls[0][1]() // xterm reports the chunk processed
    expect(session.painted.value).toBe(true)

    // Later output does not pay for the callback.
    socket.onmessage?.({ data: outputFrame('@1', '%1', 'more') })
    expect(write.mock.calls[1][1]).toBeUndefined()
  })

  it('maps window events onto the tab list', async () => {
    const { session, socket } = await attached()

    socket.onmessage?.({ data: windowFrame('added', '@3', { name: 'logs' }) })
    expect(session.tabs.value.map((tab) => tab.name)).toEqual(['agent', 'shell', 'logs'])

    socket.onmessage?.({ data: windowFrame('renamed', '@3', { name: 'tail' }) })
    expect(session.tabs.value[2].name).toBe('tail')

    socket.onmessage?.({ data: windowFrame('active-changed', '@2', { name: 'shell', active: true }) })
    expect(session.activeWindowId.value).toBe('@2')
    expect(session.tabs.value.map((tab) => tab.active)).toEqual([false, true, false])

    socket.onmessage?.({ data: windowFrame('closed', '@3', { name: 'tail' }) })
    expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@1', '@2'])
  })

  // A reconcile reports the deactivated window too, in tmux index order — so
  // selecting a lower-indexed window ends on the one that just lost the flag.
  // Following it would hand the selection straight back.
  it('ignores an active-changed for a window tmux says is not active', async () => {
    const { session, socket } = await attached()

    socket.onmessage?.({ data: windowFrame('active-changed', '@2', { name: 'shell', active: true }) })
    expect(session.activeWindowId.value).toBe('@2')

    socket.onmessage?.({ data: windowFrame('active-changed', '@1', { name: 'agent', active: true }) })
    socket.onmessage?.({ data: windowFrame('active-changed', '@2', { name: 'shell', active: false }) })

    expect(session.activeWindowId.value).toBe('@1')
    expect(session.tabs.value.map((tab) => tab.active)).toEqual([true, false])
  })

  // The webview answers neither half of xterm's default OSC 8 activation, so
  // every link kind is routed out to the OS browser instead.
  it('opens both hyperlink kinds through the system browser', async () => {
    const { session } = await attached()

    const handler = session.tabs.value[0].panes[0].term.options.linkHandler as {
      activate: (event: MouseEvent, uri: string) => void
    }
    handler.activate(new MouseEvent('click'), 'https://example.test/osc8')
    expect(wails.OpenURL).toHaveBeenCalledWith('https://example.test/osc8')

    xterm.FakeWebLinksAddon.instances[0].handler(new MouseEvent('click'), 'https://example.test/bare')
    expect(wails.OpenURL).toHaveBeenCalledWith('https://example.test/bare')
  })

  // Every client attached to a window renders the same grid, so the size a tab
  // renders at is whatever tmux reports — never what this pane measured.
  it('opens every terminal at the size tmux reported, before any output lands', async () => {
    const { session, socket } = await attached()

    for (const tab of session.tabs.value) {
      expect(tab.panes[0].term.resize).toHaveBeenCalledWith(213, 55)
      expect(tab.panes[0].term.cols).toBe(213)
      expect(tab.panes[0].term.rows).toBe(55)
    }

    // A resize re-wraps the buffer, so one after the first paint would mangle
    // the snapshot tmux just drew.
    const term = xterm.FakeTerminal.instances[1]
    socket.onmessage?.({ data: outputFrame('@2', '%2', 'from the shell') })
    expect(term.resize.mock.invocationCallOrder[0]).toBeLessThan(term.write.mock.invocationCallOrder[0])
  })

  it('follows tmux on a layout-changed event', async () => {
    const { session, socket } = await attached()

    socket.onmessage?.({ data: windowFrame('layout-changed', '@2', { name: 'shell', width: 80, height: 24 }) })

    expect(session.tabs.value[1].panes[0].term.resize).toHaveBeenLastCalledWith(80, 24)
    expect(session.tabs.value[0].panes[0].term.resize).toHaveBeenLastCalledWith(213, 55)
  })

  it('keeps a viewport following the live tail pinned through reflow', async () => {
    const { session, socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    term.buffer.active.baseY = 120
    term.buffer.active.viewportY = 120
    term.resizeEffect = () => {
      term.buffer.active.baseY = 180
      term.buffer.active.viewportY = 60
    }
    term.scrollToBottom.mockClear()

    socket.onmessage?.({
      data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, width: 100, height: 30 }),
    })

    expect(term.scrollToBottom).toHaveBeenCalledTimes(1)
    expect(term.resize.mock.invocationCallOrder.at(-1)).toBeLessThan(term.scrollToBottom.mock.invocationCallOrder[0])
    expect(term.buffer.active.viewportY).toBe(180)
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(false)
  })

  it('keeps a logical-line anchor through reflow and refreshes the tail fallback', async () => {
    const { session, socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    term.buffer.active.baseY = 120
    term.buffer.active.viewportY = 40
    term.buffer.active.cursorY = 3
    term.setLine(39, false)
    term.setLine(40, true)
    term.resizeEffect = () => {
      term.buffer.active.baseY = 160
      term.buffer.active.viewportY = 100
      term.clearLines()
      term.setLine(50, false)
      term.setLine(51, true)
      term.setLine(52, true)
      term.setLine(53, true)
      term.markers[0].line = 50
    }
    term.scrollToBottom.mockClear()

    socket.onmessage?.({
      data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, width: 100, height: 30 }),
    })

    expect(term.registerMarker).toHaveBeenCalledWith(-84)
    expect(term.scrollToLine).toHaveBeenCalledWith(52)
    expect(term.scrollToBottom).not.toHaveBeenCalled()
    expect(term.markers[0].dispose).toHaveBeenCalledTimes(1)
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(true)
  })

  it('falls back to the oldest surviving row when the anchored line is fully trimmed', async () => {
    const { session, socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    term.buffer.active.baseY = 120
    term.buffer.active.viewportY = 40
    term.buffer.active.cursorY = 3
    term.buffer.active.length = 42
    term.setLine(39, false)
    term.setLine(40, true)
    term.setLine(41, false)
    term.resizeEffect = () => {
      term.buffer.active.baseY = 100
      term.buffer.active.viewportY = 50
      term.markers[0].line = -1
      term.markers[1].line = 0
    }

    socket.onmessage?.({
      data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, width: 100, height: 30 }),
    })

    expect(term.scrollToLine).toHaveBeenCalledWith(0)
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(true)
  })

  it('takes the size off any window event, not just layout-changed', async () => {
    const { session, socket } = await attached()

    socket.onmessage?.({ data: windowFrame('renamed', '@1', { name: 'agent', active: true, width: 100, height: 30 }) })
    expect(session.tabs.value[0].panes[0].term.resize).toHaveBeenLastCalledWith(100, 30)

    // A %window-add placeholder has no size yet; the reconcile behind it does.
    socket.onmessage?.({ data: windowFrame('added', '@3', { name: '', width: 0, height: 0 }) })
    expect(xterm.FakeTerminal.instances[2].resize).toHaveBeenLastCalledWith(80, 24)
    socket.onmessage?.({ data: windowFrame('renamed', '@3', { name: 'logs', width: 213, height: 55 }) })
    expect(xterm.FakeTerminal.instances[2].resize).toHaveBeenLastCalledWith(213, 55)
  })

  it('disposes exactly the closed tab: its terminal and resize observer', async () => {
    const { session, socket } = await attached()
    mountWindow(session, '@1')
    mountWindow(session, '@2')

    socket.onmessage?.({ data: windowFrame('closed', '@2', { name: 'shell' }) })

    expect(xterm.FakeTerminal.instances[1].dispose).toHaveBeenCalledTimes(1)
    expect(FakeResizeObserver.instances[1].disconnected).toBe(true)
    expect(xterm.FakeTerminal.instances[1].onDataDisposed).toBe(true)

    expect(xterm.FakeTerminal.instances[0].dispose).not.toHaveBeenCalled()
    expect(FakeResizeObserver.instances[0].disconnected).toBe(false)
  })

  // xterm's DOM renderer cannot join box drawing or underlines across cells, so
  // an atlas renderer is the fix for #131 rather than any cell-metric tuning.
  it('loads the WebGL renderer, and only once the pane is open', async () => {
    const { session } = await attached()

    mountWindow(session, '@1')

    const term = xterm.FakeTerminal.instances[0]
    expect(xterm.FakeWebglAddon.instances).toHaveLength(1)
    expect(xterm.FakeCanvasAddon.instances).toHaveLength(0)
    const loaded = term.loadAddon.mock.calls.findIndex((call) => call[0] instanceof xterm.FakeWebglAddon)
    expect(term.open.mock.invocationCallOrder[0]).toBeLessThan(term.loadAddon.mock.invocationCallOrder[loaded])
  })

  it('falls back to the canvas renderer where WebGL2 is missing', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    xterm.FakeWebglAddon.unavailable = true
    const { session } = await attached()

    mountWindow(session, '@1')

    expect(xterm.FakeCanvasAddon.instances).toHaveLength(1)
    warn.mockRestore()
  })

  // Both atlas renderers gone leaves the DOM renderer, which draws with the
  // #131 seams — degraded, but never a pane that failed to open.
  it('still opens the pane when no renderer context exists at all', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    xterm.FakeWebglAddon.unavailable = true
    xterm.FakeCanvasAddon.unavailable = true
    const { session } = await attached()

    mountWindow(session, '@1')

    expect(xterm.FakeTerminal.instances[0].open).toHaveBeenCalledTimes(1)
    expect(session.status.value).toBe('live')
    warn.mockRestore()
  })

  it('claims the canvas renderer when a WebGL context is lost for good', async () => {
    const { session } = await attached()
    mountWindow(session, '@1')

    xterm.FakeWebglAddon.instances[0].loseContext()

    expect(xterm.FakeWebglAddon.instances[0].dispose).toHaveBeenCalledTimes(1)
    expect(xterm.FakeCanvasAddon.instances).toHaveLength(1)
  })

  // Every pooled session mounts a pane per window, so claiming a context at
  // mount spends one per background tab and walks the page past WebKit's
  // limit — where the context it costs is some other pane's. ADR terminal-renderer-claimed-on-activation.
  it('claims a renderer for the window on screen, and for the rest on activation', async () => {
    const { session } = await attached()

    mountWindow(session, '@1')
    mountWindow(session, '@2')

    expect(xterm.FakeWebglAddon.instances).toHaveLength(1)

    await session.select('@2')

    expect(xterm.FakeWebglAddon.instances).toHaveLength(2)
  })

  // The canvas claim is all that stands between a lost context and the DOM
  // renderer, so a failed one must not strand the pane there for good.
  it('retries the renderer on the next activation when the canvas claim failed', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { session } = await attached()
    mountWindow(session, '@1')
    mountWindow(session, '@2')

    xterm.FakeCanvasAddon.unavailable = true
    xterm.FakeWebglAddon.instances[0].loseContext()
    expect(xterm.FakeCanvasAddon.instances).toHaveLength(0)

    xterm.FakeCanvasAddon.unavailable = false
    await session.select('@2')
    await session.select('@1')

    expect(xterm.FakeWebglAddon.instances).toHaveLength(3)
    warn.mockRestore()
  })

  // xterm disposes its core before its addons, and a renderer addon restores a
  // renderer as it goes, so it has to be disposed while the core is still up.
  it('disposes the renderer addon ahead of the terminal it renders', async () => {
    const { session } = await attached()
    mountWindow(session, '@1')

    session.dispose()

    const addon = xterm.FakeWebglAddon.instances[0]
    const term = xterm.FakeTerminal.instances[0]
    expect(addon.dispose).toHaveBeenCalledTimes(1)
    expect(addon.dispose.mock.invocationCallOrder[0]).toBeLessThan(term.dispose.mock.invocationCallOrder[0])
  })

  // An atlas renderer caches the glyphs it rasterised, so a face that arrives
  // after the first paint stays wrong; xterm never re-measures on a font load.
  // Both weights and the italic of each: xterm measures its cell once on open
  // and the atlas caches whatever was resident, so a face arriving later stays
  // wrong until the atlas is cleared. ADR terminal-atlas-renderer.
  it('preloads every face a pane can draw with before it attaches', async () => {
    const { client } = await attached()

    const stack = terminalFontStack('')
    const px = defaultTerminalFontSizePx
    expect(loadedFaces).toEqual([
      `${defaultTerminalFontWeight} ${px}px ${stack}`,
      `italic ${defaultTerminalFontWeight} ${px}px ${stack}`,
      `${defaultTerminalFontWeightBold} ${px}px ${stack}`,
      `italic ${defaultTerminalFontWeightBold} ${px}px ${stack}`,
      // The symbol face, warmed by name because a stack would resolve to the
      // text face and leave this one unfetched.
      `${defaultTerminalFontWeight} ${px}px '${SYMBOL_FONT}'`,
    ])
    const load = document.fonts.load as ReturnType<typeof vi.fn>
    expect(load.mock.invocationCallOrder.at(-1)).toBeLessThan(client.attach.mock.invocationCallOrder[0])
  })

  it('opens every pane at the configured line height and letter spacing', async () => {
    await attached()

    for (const term of xterm.FakeTerminal.instances) {
      expect(term.options.lineHeight).toBe(defaultTerminalLineHeight)
      expect(term.options.letterSpacing).toBe(defaultTerminalLetterSpacing)
    }
  })

  it('re-applies line height and letter spacing to open panes', async () => {
    await attached()

    useTerminalFont().setLineHeight(1.4)
    useTerminalFont().setLetterSpacing(2)
    await flushPromises()

    for (const term of xterm.FakeTerminal.instances) {
      expect(term.options.lineHeight).toBe(1.4)
      expect(term.options.letterSpacing).toBe(2)
    }
  })

  it('sends typed input as an input frame for the typing pane', async () => {
    const { session, socket } = await attached()

    xterm.FakeTerminal.instances[1].type('ls\r')

    expect(socket.sent).toHaveLength(1)
    expect(Array.from(socket.sent[0])).toEqual([0x10, 2, 0x25, 0x32, 0x6c, 0x73, 0x0d])
    expect(session.status.value).toBe('live')
  })

  it('splits input larger than the frame cap across frames', async () => {
    const { socket } = await attached()

    xterm.FakeTerminal.instances[0].type('x'.repeat(5000))

    expect(socket.sent).toHaveLength(2)
    for (const frame of socket.sent) expect(frame.length).toBeLessThanOrEqual(4096)
    const total = socket.sent.reduce((sum, frame) => sum + frame.length - 4, 0)
    expect(total).toBe(5000)
  })

  // The measurement is a vote sent to tmux, which may or may not honour it: it
  // must not touch the grid any terminal renders at.
  it('votes the measured size on a resize observation without resizing anything', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()

    const host = mountWindow(session, '@1')
    resizeHost(host, 120, 40)
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)

    expect(client.resize).toHaveBeenCalledWith('hive-abc', 120, 40)
    for (const term of xterm.FakeTerminal.instances) {
      expect(term.resize).toHaveBeenLastCalledWith(213, 55)
    }
  })

  it('votes from the window box, not a pane box', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()

    session.attachTab('@1', paneHostOf(120, 40))
    session.attachPane('%1', paneHostOf(60, 20))
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)

    expect(client.resize).toHaveBeenCalledWith('hive-abc', 120, 40)
    expect(client.resize).not.toHaveBeenCalledWith('hive-abc', 60, 20)
  })

  // Only the shown window's panes claim an atlas renderer; the DOM renderer
  // behind a hidden one keeps the device width unfloored, so its cell differs.
  // tmux order puts the hidden window first, and it opens first.
  it('measures the cell and votes off the shown window, not the first window opened', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    client.attach.mockResolvedValue({
      windows: [
        {
          windowId: '@1',
          name: 'agent',
          active: false,
          activePane: '%1',
          width: 213,
          height: 55,
          zoomed: false,
          layout: leaf('%1', 213, 55),
        },
        {
          windowId: '@2',
          name: 'shell',
          active: true,
          activePane: '%2',
          width: 213,
          height: 55,
          zoomed: false,
          layout: leaf('%2', 213, 55),
        },
      ],
    })
    const session = open(client)
    await session.start()
    await flushPromises()
    xterm.FakeTerminal.instances[0]._core._renderService!.dimensions.css.cell = { width: 7.8, height: 25 }

    mountWindow(session, '@1', paneHostOf(120, 40))
    mountWindow(session, '@2', paneHostOf(120, 40))
    FakeResizeObserver.instances[1].trigger()
    await vi.advanceTimersByTimeAsync(100)

    expect(session.activeWindowId.value).toBe('@2')
    expect(session.cell.value).toEqual({ width: 10, height: 25 })
    expect(client.resize).toHaveBeenCalledWith('hive-abc', 120, 40)
    expect(client.resize).not.toHaveBeenCalledWith('hive-abc', 153, 40)
  })

  // A size preset changes cell metrics, not the host box, so the observer never
  // fires: the composable itself must re-vote. The grid still belongs to tmux.
  it('applies a font size preset to every open terminal and re-votes', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()
    const host = mountWindow(session, '@1')
    await vi.advanceTimersByTimeAsync(100)
    client.resize.mockClear()
    resizeHost(host, 100, 30)

    useTerminalFont().setFontSize(16)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(100)

    for (const term of xterm.FakeTerminal.instances) {
      expect(term.options.fontSize).toBe(16)
    }
    expect(client.resize).toHaveBeenCalledWith('hive-abc', 100, 30)
    for (const term of xterm.FakeTerminal.instances) {
      expect(term.resize).toHaveBeenLastCalledWith(213, 55)
    }
  })

  // #181: normal cells were locked to whatever the renderer drew, with no way
  // to ask for a lighter face.
  it('opens every terminal at the configured font weights', async () => {
    await attached()

    for (const term of xterm.FakeTerminal.instances) {
      expect(term.options.fontWeight).toBe(defaultTerminalFontWeight)
      expect(term.options.fontWeightBold).toBe(defaultTerminalFontWeightBold)
    }
  })

  // Weight moves the advance width the same way size does, so the grid has to
  // be re-voted for — and the face has to be resident before xterm re-measures
  // against it, or it measures the outgoing one.
  it('applies a font weight to every open terminal and re-votes', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()
    const host = mountWindow(session, '@1')
    await vi.advanceTimersByTimeAsync(100)
    client.resize.mockClear()
    loadedFaces.length = 0
    resizeHost(host, 100, 30)

    useTerminalFont().setFontWeight(700)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(100)

    for (const term of xterm.FakeTerminal.instances) {
      expect(term.options.fontWeight).toBe(700)
    }
    expect(loadedFaces).toContain(`700 ${defaultTerminalFontSizePx}px ${terminalFontStack('')}`)
    expect(client.resize).toHaveBeenCalledWith('hive-abc', 100, 30)
  })

  // A chosen family leads the stack and the bundled face backs it, so a font
  // the OS reports but the webview cannot resolve degrades to the shipped one
  // rather than to whatever `monospace` happens to be.
  it('applies a font family to every open terminal, keeping the bundled face as fallback', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()
    mountWindow(session, '@1')
    await vi.advanceTimersByTimeAsync(100)

    useTerminalFont().setFontFamily('Menlo')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(100)

    for (const term of xterm.FakeTerminal.instances) {
      expect(term.options.fontFamily).toBe(terminalFontStack('Menlo'))
      expect(term.options.fontFamily).toContain(TERMINAL_FONT)
    }
  })

  // The attach size is a vote tmux obeys, so attaching with a placeholder would
  // resize the session — and every other client attached to it — to a size
  // nothing had measured. Setting no size leaves it alone.
  it('attaches without a size when nothing has been measured', async () => {
    const client = fakeClient()
    await open(client).start()

    expect(client.attach).toHaveBeenCalledWith('hive-abc', 0, 0)
  })

  it('attaches with the size this app window last voted, and does not re-vote it', async () => {
    vi.useFakeTimers()
    localStorage.setItem(
      'hive.terminal.vote',
      JSON.stringify({ cols: 120, rows: 40, metrics: useTerminalFont().cellMetrics() }),
    )
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()

    expect(client.attach).toHaveBeenCalledWith('hive-abc', 120, 40)

    mountWindow(session, '@1', paneHostOf(120, 40))
    await vi.advanceTimersByTimeAsync(100)

    expect(client.resize).not.toHaveBeenCalled()
  })

  it('remembers a measured vote for the next session it attaches', async () => {
    vi.useFakeTimers()
    const first = fakeClient()
    const session = open(first)
    await session.start()
    await flushPromises()
    mountWindow(session, '@1', paneHostOf(213, 55))
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)

    const second = fakeClient()
    await open(second).start()

    expect(second.attach).toHaveBeenCalledWith('hive-abc', 213, 55)
  })

  // A vote tmux never granted means another attached client decided the size.
  // Nothing announces that, so the difference is the only signal there is.
  it('reports the size tmux granted when it does not match the vote', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()
    mountWindow(session, '@1', paneHostOf(300, 80))

    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)
    expect(session.sizeConstraint.value).toBeNull()

    // tmux answers a vote it honours with a window event; silence past the
    // settle window is what says some other client won.
    await vi.advanceTimersByTimeAsync(1000)
    expect(session.sizeConstraint.value).toEqual({ voted: { cols: 300, rows: 80 }, granted: { cols: 213, rows: 55 } })

    session.dismissSizeConstraint()
    expect(session.sizeConstraint.value).toBeNull()
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(1000)
    expect(session.sizeConstraint.value).toBeNull()
  })

  it('clears the constraint once tmux grants the voted size', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()
    sockets[0].onopen?.()
    mountWindow(session, '@1', paneHostOf(300, 80))
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(1100)
    expect(session.sizeConstraint.value).not.toBeNull()

    sockets[0].onmessage?.({
      data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, width: 300, height: 80 }),
    })
    await vi.advanceTimersByTimeAsync(1000)

    expect(session.sizeConstraint.value).toBeNull()
  })

  it('votes nothing when the pane cannot be measured', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()

    mountWindow(session, '@1')
    // The fake uses xterm's pre-measurement 0×0 cell dimensions.
    xterm.FakeTerminal.instances[0]._core._renderService!.dimensions.css.cell = { width: 0, height: 0 }
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)

    expect(client.resize).not.toHaveBeenCalled()
    warn.mockRestore()
  })

  // The fake mirrors a private xterm cell field; this warning catches upstream
  // changes that would otherwise silently stop size votes.
  it('warns once when an opened, visible pane reports no cell', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()
    for (const term of xterm.FakeTerminal.instances) term._core = {}

    mountWindow(session, '@1')
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)

    expect(client.resize).not.toHaveBeenCalled()
    const warnings = warn.mock.calls.filter((call) => String(call[0]).includes('terminalGrid.ts'))
    expect(warnings).toHaveLength(1)
    warn.mockRestore()
  })

  // Pooled sessions legitimately expose 0×0 pane boxes under display:none.
  it('does not warn about a pane with no rendered box', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { session } = await attached()
    for (const term of xterm.FakeTerminal.instances) term._core = {}

    mountWindow(session, '@1', document.createElement('div'))

    expect(warn).not.toHaveBeenCalled()
    warn.mockRestore()
  })

  // A pooled session's windows sit behind display:none while another session
  // is shown, and a hidden host has no client box. Proposing a grid from one
  // would squeeze the session's windows for every client attached to them.
  it('never votes from a pane with no rendered box', async () => {
    vi.useFakeTimers()
    const client = fakeClient()
    const session = open(client)
    await session.start()
    await flushPromises()

    mountWindow(session, '@1', document.createElement('div'))
    FakeResizeObserver.instances[0].trigger()
    await vi.advanceTimersByTimeAsync(100)

    expect(client.resize).not.toHaveBeenCalled()
  })

  it('ends on a lifecycle exit with the reason tmux gave', async () => {
    const { session, socket } = await attached()
    expect(session.tabs.value).toHaveLength(2)

    socket.onmessage?.({ data: jsonFrame(0x02, { kind: 'exited', windowId: '', message: 'overflow' }) })

    // What tmux said, before the probe that settles what it meant.
    expect(session.status.value).toBe('ended')
    expect(session.endReason.value).toBe('exited')
    expect(session.error.value).toBe('overflow')
    expect(socket.closed).toBe(true)
  })

  // Overflow used to end the stream, which cost the user every tab, every
  // scroll position and their focus. Degraded says the same thing without any
  // of that: the panes stay, the socket stays, and the repaint already on the
  // wire is what they are painted from.
  it('keeps the view and repaints it in place when the stream degrades', async () => {
    const { session, socket } = await attached()
    socket.onmessage?.({ data: outputFrame('@1', '%1', 'before the flood') })

    socket.onmessage?.({ data: jsonFrame(0x02, { kind: 'degraded', windowId: '', message: 'overflow' }) })

    expect(session.status.value).toBe('live')
    expect(session.endReason.value).toBeNull()
    expect(session.tabs.value).toHaveLength(2)
    expect(socket.closed).toBe(false)
    // Both panes, not just the streaming one: the backlog is the session's and
    // the repaint behind it covers every window.
    for (const term of xterm.FakeTerminal.instances) expect(term.reset).toHaveBeenCalled()

    socket.onmessage?.({ data: outputFrame('@1', '%1', 'REPAINTED') })
    const write = session.tabs.value[0].panes[0].term.write as ReturnType<typeof vi.fn>
    const written = write.mock.calls[write.mock.calls.length - 1][0] as Uint8Array
    expect(new TextDecoder().decode(written)).toBe('REPAINTED')
  })

  // The notice is the whole point: a recovery that hides the gap is worse than
  // the teardown it replaced, which at least told the truth loudly.
  it('raises a dismissible dropped-output notice for a degraded stream', async () => {
    const { session, socket } = await attached()
    expect(session.outputDropped.value).toBe(false)

    socket.onmessage?.({ data: jsonFrame(0x02, { kind: 'degraded', windowId: '', message: 'overflow' }) })
    expect(session.outputDropped.value).toBe(true)

    session.dismissOutputDropped()
    expect(session.outputDropped.value).toBe(false)
  })

  it('retains confirmed termination and the control-client report when tmux is gone', async () => {
    const { session, socket } = await attached()

    socket.onmessage?.({ data: jsonFrame(0x02, { kind: 'exited', windowId: '', message: 'overflow' }) })
    await flushPromises()

    expect(session.endReason.value).toBe('terminated')
    expect(session.error.value).toContain('Tmux session terminated. No exit details are available.')
    expect(session.error.value).toContain('Terminal connection report:\noverflow')
    expect(session.tabs.value).toHaveLength(0)
  })

  it('keeps an exit as an error when tmux is still holding the session', async () => {
    const client = fakeClient()
    client.listWindows.mockResolvedValue({
      'hive-abc': [{ windowId: '@1', name: 'agent', active: true, width: 213, height: 55 }],
    })
    const { session, socket } = await attached(client)

    socket.onmessage?.({ data: jsonFrame(0x02, { kind: 'exited', windowId: '', message: 'overflow' }) })
    await flushPromises()

    // Only this view's control client went away; reconnecting is the way back.
    expect(session.endReason.value).toBe('exited')
    expect(session.error.value).toBe('overflow')
    expect(session.tabs.value).toHaveLength(2)
  })

  it('leaves the exit standing when the probe itself fails', async () => {
    const client = fakeClient()
    client.listWindows.mockRejectedValue(new Error('tmux is gone'))
    const { session, socket } = await attached(client)

    socket.onmessage?.({ data: jsonFrame(0x02, { kind: 'exited', windowId: '', message: 'overflow' }) })
    await flushPromises()

    expect(session.endReason.value).toBe('exited')
    expect(session.error.value).toBe('overflow')
  })

  it('ends on a transport drop as a distinct signal', async () => {
    const { session, socket } = await attached()

    socket.onclose?.()

    expect(session.status.value).toBe('ended')
    expect(session.endReason.value).toBe('disconnected')
    expect(session.error.value).toBe('Terminal connection lost. The tmux session may still be running.')
  })

  it('ends when the attach itself fails', async () => {
    const client = fakeClient()
    client.attach.mockRejectedValue(new Error('no terminal is attached for that slug'))
    const session = open(client)

    await session.start()

    expect(session.status.value).toBe('ended')
    expect(session.endReason.value).toBe('attach-failed')
    expect(session.error.value).toBe('no terminal is attached for that slug')
  })

  it('separates a session that is not running from an attach that failed', async () => {
    const client = fakeClient()
    client.attach.mockRejectedValue(new TerminalRequestError('session "hive-abc" is not running', 'not_found'))
    const session = open(client)

    await session.start()

    // Not a fault: the view turns this into the panel offering to start it.
    expect(session.endReason.value).toBe('not-started')
    expect(session.error.value).toBe('session "hive-abc" is not running')
  })

  it('reports termination without diagnostics after a dropped stream and an absent listing', async () => {
    const { session, socket } = await attached()
    socket.onclose?.()
    await flushPromises()
    expect(session.endReason.value).toBe('terminated')
    expect(session.error.value).toBe('Tmux session terminated. No exit details are available.')
    expect(session.tabs.value).toHaveLength(0)
  })

  it('keeps connection loss uncertain if the listing fails', async () => {
    const client = fakeClient()
    client.listWindows.mockRejectedValue(new Error('probe failed'))
    const { session, socket } = await attached(client)
    socket.onclose?.()
    await flushPromises()
    expect(session.endReason.value).toBe('disconnected')
    expect(session.error.value).toContain('may still be running')
    expect(session.tabs.value).toHaveLength(2)
  })

  it('retains termination across terminal disposal and a failed reattach', async () => {
    const { client, session, socket } = await attached()
    socket.onclose?.()
    await flushPromises()
    session.dispose()
    client.attach.mockRejectedValue(new TerminalRequestError('not running', 'not_found'))
    const reopened = open(client)
    await reopened.start()
    expect(reopened.endReason.value).toBe('terminated')
    expect(reopened.error.value).toContain('No exit details are available')
  })

  it('retains multiline launch failures until a successful attach', async () => {
    const report = 'Launch failed\n\n  indented detail\nCleanup incomplete: @2'
    useTerminalOutcomes().report('hive-abc', { reason: 'start-failed', detail: report })
    const client = fakeClient()
    client.attach.mockRejectedValueOnce(new TerminalRequestError('not running', 'not_found'))
    const session = open(client)
    await session.start()
    expect(session.endReason.value).toBe('start-failed')
    expect(session.error.value).toBe(report)
    await session.reconnect()
    sockets[sockets.length - 1].onopen?.()
    expect(session.endReason.value).toBeNull()
    expect(useTerminalOutcomes().outcome('hive-abc')).toBeNull()
  })

  it('keeps intentional stopping distinct from unknown termination', async () => {
    const { session, socket } = await attached()
    useTerminalOutcomes().report('hive-abc', { reason: 'stopped', detail: 'Stopped by you.' })
    socket.onclose?.()
    await flushPromises()
    expect(session.endReason.value).toBe('stopped')
    expect(session.error.value).toBe('Stopped by you.')
  })

  it('does not let an older reconciliation replace a successful reconnect', async () => {
    const client = fakeClient()
    let finishProbe!: (windows: Record<string, never[]>) => void
    client.listWindows.mockImplementation(
      () =>
        new Promise((resolve) => {
          finishProbe = resolve
        }),
    )
    const { session, socket } = await attached(client)
    socket.onclose?.()
    await session.reconnect()
    sockets[1].onopen?.()
    finishProbe({})
    await flushPromises()
    expect(session.status.value).toBe('live')
    expect(session.endReason.value).toBeNull()
    expect(session.tabs.value).toHaveLength(2)
  })

  it('drops a window that closed on its own without ending its sibling or raising an error', async () => {
    const { session, socket } = await attached()
    socket.onmessage?.({ data: jsonFrame(0x01, { kind: 'closed', windowId: '@1' }) })
    expect(session.status.value).toBe('live')
    expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@2'])
    expect(session.actionError.value).toBeFalsy()
  })

  it('reconnect re-attaches with fresh terminals', async () => {
    const { client, session, socket } = await attached()
    socket.onerror?.()
    const before = session.tabs.value.map((tab) => tab.panes[0].term)

    await session.reconnect()
    await flushPromises()
    sockets[1].onopen?.()

    expect(client.attach).toHaveBeenCalledTimes(2)
    expect(session.status.value).toBe('live')
    expect(xterm.FakeTerminal.instances).toHaveLength(4)
    for (const term of before) expect(term.dispose).toHaveBeenCalled()
    expect(session.tabs.value.every((tab) => !before.includes(tab.panes[0].term))).toBe(true)
  })

  it('detaches and tears everything down on dispose', async () => {
    const { client, session, socket } = await attached()
    mountWindow(session, '@1')

    session.dispose()

    expect(client.detach).toHaveBeenCalledWith('hive-abc')
    expect(socket.closed).toBe(true)
    expect(session.tabs.value).toHaveLength(0)
    expect(FakeResizeObserver.instances[0].disconnected).toBe(true)
    for (const term of xterm.FakeTerminal.instances) expect(term.dispose).toHaveBeenCalled()
  })

  it('drives window operations through the control plane', async () => {
    const { client, session } = await attached()

    await session.select('@2')
    await session.newWindow()
    await session.rename('@2', 'build')
    await session.closeWindow('@2')

    expect(client.selectWindow).toHaveBeenCalledWith('hive-abc', '@2')
    expect(client.newWindow).toHaveBeenCalledWith('hive-abc')
    expect(client.renameWindow).toHaveBeenCalledWith('hive-abc', '@2', 'build')
    expect(client.closeWindow).toHaveBeenCalledWith('hive-abc', '@2')
  })

  it('activates and focuses a window it created, once tmux announces it', async () => {
    const { session, socket } = await attached()

    await session.newWindow()
    socket.onmessage?.({ data: windowFrame('added', '@3', { name: 'logs' }) })
    await flushPromises()

    expect(session.activeWindowId.value).toBe('@3')
    // A window this view asked for is one to type in.
    expect(xterm.FakeTerminal.instances.at(-1)?.focus).toHaveBeenCalled()
  })

  it.each(['before', 'after'])(
    'activates a peer agent when tmux announces it %s the launch response',
    async (order) => {
      const { client, session, socket } = await attached()
      let resolveLaunch!: (value: { windowId: string }) => void
      client.newAgentWindow.mockReturnValue(
        new Promise((resolve) => {
          resolveLaunch = resolve
        }),
      )
      const launch = session.newAgentWindow('codex')
      if (order === 'before') socket.onmessage?.({ data: windowFrame('added', '@4', { name: 'codex' }) })
      resolveLaunch({ windowId: '@4' })
      await launch
      if (order === 'after') socket.onmessage?.({ data: windowFrame('added', '@4', { name: 'codex' }) })
      await flushPromises()
      expect(client.newAgentWindow).toHaveBeenCalledExactlyOnceWith('hive-abc', 'codex')
      expect(session.activeWindowId.value).toBe('@4')
      expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@1', '@2', '@4'])
      expect(socket.sent).toHaveLength(0)
      expect(xterm.FakeTerminal.instances.at(-1)?.focus).toHaveBeenCalled()
    },
  )

  it('reports a failed peer launch and keeps the existing terminal active', async () => {
    const { client, session } = await attached()
    client.newAgentWindow.mockRejectedValue(new Error('profile launch refused'))
    await session.newAgentWindow('codex')
    expect(session.actionError.value).toContain('profile launch refused')
    expect(session.activeWindowId.value).toBe('@1')
    expect(client.newWindow).not.toHaveBeenCalled()
  })

  // tmux announces the window before it has a layout, and the line has to be
  // addressed to a pane, so it waits for the event that names one.
  it('types the command for a new window into the pane tmux announces for it', async () => {
    const { socket, session } = await attached()

    await session.newWindow('make')
    socket.onmessage?.({
      data: windowFrame('added', '@3', { name: '', activePane: '', width: 0, height: 0, layout: null }),
    })
    expect(socket.sent).toHaveLength(0)

    socket.onmessage?.({ data: windowFrame('layout-changed', '@3', { name: 'make', activePane: '%7' }) })

    expect(socket.sent).toHaveLength(1)
    expect(Array.from(socket.sent[0])).toEqual([0x10, 2, 0x25, 0x37, 0x6d, 0x61, 0x6b, 0x65, 0x0d])

    // Reconciliation reports the window again; it must not replay the command.
    socket.onmessage?.({ data: windowFrame('renamed', '@3', { name: 'make', activePane: '%7' }) })
    expect(socket.sent).toHaveLength(1)
  })

  // Another client's window must not pull the keyboard out of the pane in front
  // of the user, so it becomes active without taking focus.
  it('leaves focus alone for a window another client opened', async () => {
    const { session, socket } = await attached()

    socket.onmessage?.({ data: windowFrame('added', '@3', { name: 'logs', active: true }) })
    await flushPromises()

    expect(session.activeWindowId.value).toBe('@3')
    expect(xterm.FakeTerminal.instances.at(-1)?.focus).not.toHaveBeenCalled()
  })

  it('refocuses the pane when the active window is reselected', async () => {
    const { client, session } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    term.focus.mockClear()

    await session.select('@1')

    expect(term.focus).toHaveBeenCalled()
    expect(client.selectWindow).not.toHaveBeenCalled()
  })

  it('tracks whether the active window is scrolled above the live tail', async () => {
    const { session } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(false)
    term.scrollTo(5, 12)
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(true)

    // Entering the alternate screen (a full-screen TUI) has no scrollback and
    // fires no scroll event, so the buffer switch is what clears the flag.
    term.buffer.active = {
      type: 'alternate',
      cursorY: 0,
      viewportY: 0,
      baseY: 0,
      length: 0,
      getLine: () => undefined,
    }
    term.switchBuffer()
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(false)
  })

  // A wheel notch is about three rows. Offering the way back the moment the
  // viewport moves at all would put a pill on screen for a nudge, and take it
  // away again before it finished appearing.
  it('waits for a scroll worth calling a scroll before offering the way back', async () => {
    const { session } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    term.scrollTo(96, 100)
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(false)

    term.scrollTo(94, 100)
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(true)
  })

  it('scrolls the active window back to the tail and refocuses it', async () => {
    const { session } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    term.scrollTo(5, 12)
    term.focus.mockClear()

    session.scrollToBottom()

    expect(term.scrollToBottom).toHaveBeenCalled()
    expect(term.focus).toHaveBeenCalled()
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(false)
  })

  describe('reordering windows', () => {
    // tmux reports the whole order back, and it is the one that lands: this
    // client's guess is only what fills the gap while the round trip runs.
    it('reorders on the drop and settles on the order tmux answers with', async () => {
      const { client, session } = await attached()
      let resolve: (value: { windows: { windowId: string }[] }) => void = () => {}
      client.moveWindow.mockReturnValue(
        new Promise((r) => {
          resolve = r
        }),
      )

      const moving = session.moveWindow('@1', 1)
      expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@2', '@1'])
      expect(client.moveWindow).toHaveBeenCalledWith('hive-abc', '@1', 1)

      resolve({ windows: [{ windowId: '@1' }, { windowId: '@2' }] })
      await moving

      expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@1', '@2'])
      expect(session.actionError.value).toBeNull()
    })

    it('keeps the terminals and the selection across a move', async () => {
      const { client, session } = await attached()
      client.moveWindow.mockResolvedValue({ windows: [{ windowId: '@2' }, { windowId: '@1' }] })
      const terminals = session.tabs.value.map((tab) => tab.panes[0].term)

      await session.moveWindow('@1', 1)

      expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@2', '@1'])
      expect(session.tabs.value.map((tab) => tab.panes[0].term)).toEqual([terminals[1], terminals[0]])
      expect(session.activeWindowId.value).toBe('@1')
      expect(terminals[0].dispose).not.toHaveBeenCalled()
    })

    it('puts the strip back and names the failure when tmux refuses', async () => {
      const { client, session } = await attached()
      client.moveWindow.mockRejectedValue(new Error('tmux refused the move'))

      await session.moveWindow('@1', 1)

      expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@1', '@2'])
      expect(session.actionError.value).toBe('tmux refused the move')
      expect(session.status.value).toBe('live')
    })

    // A window opened while the move was in flight is tmux's news, not part of
    // what the refusal was about.
    it('rolls back the order without dropping a window that arrived meanwhile', async () => {
      const { client, session, socket } = await attached()
      let reject: (reason: Error) => void = () => {}
      client.moveWindow.mockReturnValue(
        new Promise((_, r) => {
          reject = r
        }),
      )

      const moving = session.moveWindow('@1', 1)
      socket.onmessage?.({ data: windowFrame('added', '@3', { name: 'logs' }) })
      reject(new Error('tmux refused the move'))
      await moving

      expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@1', '@2', '@3'])
    })

    it('never asks tmux for a move that changes nothing', async () => {
      const { client, session } = await attached()

      await session.moveWindow('@1', 0)
      await session.moveWindow('@1', 5)
      await session.moveWindow('@404', 1)

      expect(client.moveWindow).not.toHaveBeenCalled()
      expect(session.tabs.value.map((tab) => tab.windowId)).toEqual(['@1', '@2'])
    })
  })

  it('reports a failed control action without ending the session', async () => {
    const { client, session } = await attached()
    client.newWindow.mockRejectedValue(new Error('tmux refused'))

    await session.newWindow()

    expect(session.actionError.value).toBe('tmux refused')
    expect(session.status.value).toBe('live')
  })

  // xterm suppresses onScroll for a wheel, a trackpad and a dragged scrollbar:
  // its viewport syncs the buffer from the DOM scroll and swallows the event.
  // Without the DOM listener nothing notices the viewport leaving the tail on an
  // idle session, and the way back is never offered.
  it('offers the way back to the tail when the user scrolls, which xterm does not announce', async () => {
    const { session } = await attached()
    const host = paneHost()
    mountWindow(session, '@1', host)
    const term = xterm.FakeTerminal.instances[0]
    const viewport = host.querySelector('.xterm-viewport')!

    term.buffer.active.viewportY = 40
    term.buffer.active.baseY = 120
    viewport.dispatchEvent(new Event('scroll'))
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(true)

    term.buffer.active.viewportY = 120
    viewport.dispatchEvent(new Event('scroll'))
    expect(session.tabs.value[0].panes[0].scrolledUp).toBe(false)
  })

  it('searches the active window and reports the hit it is on', async () => {
    const { session } = await attached()
    xterm.FakeSearchAddon.results = { resultIndex: 2, resultCount: 9 }

    session.openSearch()
    session.setSearchQuery('panic')

    expect(session.search.value.open).toBe(true)
    // resultIndex is 0-based; the bar counts from 1.
    expect(session.search.value).toMatchObject({ query: 'panic', matches: 9, index: 3 })
    const finder = xterm.FakeSearchAddon.instances[0]
    expect(finder.calls.at(-1)).toMatchObject({ mode: 'next', term: 'panic' })

    session.findPrevious()
    expect(finder.calls.at(-1)).toMatchObject({ mode: 'previous', term: 'panic' })
  })

  // A hit count is only true of the buffer it was counted in, so carrying one
  // across tabs would put a number on the new window that never matched it.
  it('re-runs the search against the window it switches to', async () => {
    const { session } = await attached()
    session.openSearch()
    session.setSearchQuery('panic')

    await session.select('@2')

    const [first, second] = xterm.FakeSearchAddon.instances
    expect(first.clearDecorations).toHaveBeenCalled()
    expect(second.calls.at(-1)).toMatchObject({ term: 'panic' })
  })

  // terminal.find opens the bar from App.vue's dispatcher; the pane only has to
  // leave the chord alone, and keep readline's Ctrl+F.
  it('keeps the find chord off the wire', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    expect(term.press({ key: 'f', ctrlKey: true })).toBe(true)
    expect(term.press({ key: 'f', metaKey: true })).toBe(false)
    expect(term.press({ key: 'F', ctrlKey: true, shiftKey: true })).toBe(false)
    expect(socket.sent).toHaveLength(0)
  })

  it('declines an escape chord only once something is bound to it', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    const keys = useKeybindings()

    expect(term.press({ key: 'i', metaKey: true })).toBe(true)

    keys.addBinding('view.go-inbox', 'mod+i')
    expect(term.press({ key: 'i', metaKey: true })).toBe(false)
    expect(term.press({ key: 'i' })).toBe(true)
    expect(socket.sent).toHaveLength(0)
  })

  // The pane cannot tell whether the key after ⌘G finishes the binding, so it
  // declines every key while the sequence is pending.
  it('declines the keys of a sequence the escape chord starts', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    const keys = useKeybindings()
    keys.addBinding('view.go-inbox', 'mod+g i')

    expect(term.press({ key: 'g', metaKey: true })).toBe(false)
    keys.pendingSequence.value = { steps: ['mod+g'], continuations: [{ step: 'i', commandId: 'view.go-inbox' }] }
    expect(term.press({ key: 'i' })).toBe(false)

    keys.clearPendingSequence()
    expect(term.press({ key: 'i' })).toBe(true)
    expect(socket.sent).toHaveLength(0)
  })

  // xterm writes the same CR for Shift+Enter as for Enter, and Pi binds its
  // newline to the CSI u form the key has nowhere else (#538).
  it('forwards Shift+Enter as a CSI u modified-enter rather than a carriage return', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    expect(term.press({ key: 'Enter', shiftKey: true })).toBe(false)
    expect(socket.sent.map((frame) => Array.from(frame))).toEqual([
      [0x10, 2, 0x25, 0x31, 0x1b, 0x5b, 0x31, 0x33, 0x3b, 0x32, 0x75],
    ])

    // A bare Enter stays xterm's, which writes the CR itself.
    expect(term.press({ key: 'Enter' })).toBe(true)
    expect(socket.sent).toHaveLength(1)
  })

  // Once a rebind puts a pane-piercing command on shift+enter the chord is
  // App.vue's, and the pane must write neither the CSI u nor the CR.
  it('keeps Shift+Enter off the wire once a pane-piercing command is bound to it', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    useKeybindings().addBinding('tasks.toggle', 'shift+enter')
    expect(term.press({ key: 'Enter', shiftKey: true })).toBe(false)
    expect(socket.sent).toHaveLength(0)
  })

  // App.vue acts on the chord from its own window listener; declining it here is
  // only about keeping xterm from *also* writing an arrow escape to the pane.
  it('keeps the chord back to the session tree off the wire', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    expect(term.press({ key: 'ArrowLeft', metaKey: true })).toBe(false)
    // A bare arrow is the pane's, and tmux needs it.
    expect(term.press({ key: 'ArrowLeft' })).toBe(true)
    expect(socket.sent).toHaveLength(0)
  })

  // Ctrl+2 is a NUL on a platform without Command, so the pane would write the
  // jump chord to tmux as well as App.vue acting on it.
  it('keeps a numbered window jump off the wire', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    expect(term.press({ key: '2', metaKey: true })).toBe(false)
    expect(term.press({ key: '2', ctrlKey: true })).toBe(false)
    // A bare digit is text.
    expect(term.press({ key: '2' })).toBe(true)
    expect(socket.sent).toHaveLength(0)
  })

  // The tab chords escape rather than pierce, so the pane keeps the bare Ctrl
  // forms — Ctrl+T is readline's transpose-chars, Ctrl+W its unix-word-rubout.
  it('keeps the window lifecycle chords off the wire without taking their readline forms', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    expect(term.press({ key: 't', metaKey: true })).toBe(false)
    expect(term.press({ key: 'W', ctrlKey: true, shiftKey: true })).toBe(false)
    expect(term.press({ key: '}', metaKey: true, shiftKey: true })).toBe(false)

    expect(term.press({ key: 't', ctrlKey: true })).toBe(true)
    expect(term.press({ key: 'w', ctrlKey: true })).toBe(true)
    expect(socket.sent).toHaveLength(0)
  })

  it.each(['Macintosh', 'Windows', 'Linux'])('keeps the sidebar shortcut off the wire on %s', async (userAgent) => {
    Object.defineProperty(navigator, 'userAgent', { configurable: true, value: userAgent })
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]

    expect(term.press({ key: 'b', metaKey: true })).toBe(false)
    expect(term.press({ key: 'b', ctrlKey: true })).toBe(userAgent === 'Macintosh')
    expect(term.press({ key: 'b' })).toBe(true)
    expect(socket.sent).toHaveLength(0)
  })

  // Alt+T is readline's transpose-words, and nothing here may take it from the
  // shell until a command that pierces the pane is actually bound to it — the
  // decision the pane and App.vue both read off the catalog flag.
  it('declines an alt chord only once a pane-piercing command claims it', async () => {
    const { socket } = await attached()
    const term = xterm.FakeTerminal.instances[0]
    const keys = useKeybindings()

    expect(term.press({ key: 't', altKey: true })).toBe(true)

    // report.open reaches the pane by neither route, so binding it changes
    // nothing for the shell.
    keys.addBinding('report.open', 'alt+t')
    expect(term.press({ key: 't', altKey: true })).toBe(true)

    keys.removeBinding('report.open', 'alt+t')
    keys.addBinding('tasks.toggle', 'alt+t')
    expect(term.press({ key: 't', altKey: true })).toBe(false)
    expect(socket.sent).toHaveLength(0)
  })

  // Arrowing down the tree walks through windows; a pane that took focus on the
  // way past would send the next arrow to tmux instead of the sidebar.
  it('does not focus the pane when a window is selected by keyboard', async () => {
    const { session } = await attached()
    const [first, second] = xterm.FakeTerminal.instances

    paneMayAutoFocus.value = false
    await session.select('@2')
    expect(second.focus).not.toHaveBeenCalled()

    paneMayAutoFocus.value = true
    await session.select('@1')
    expect(first.focus).toHaveBeenCalled()
  })

  it('drops the query and the highlights when the bar closes', async () => {
    const { session } = await attached()
    session.openSearch()
    session.setSearchQuery('panic')

    session.closeSearch()

    expect(session.search.value).toEqual({ open: false, query: '', matches: 0, index: 0 })
    expect(xterm.FakeSearchAddon.instances[0].clearDecorations).toHaveBeenCalled()
    expect(xterm.FakeTerminal.instances[0].focus).toHaveBeenCalled()
  })

  describe('panes', () => {
    const split = (): PaneLayout => ({
      split: 'leftright',
      x: 0,
      y: 0,
      width: 213,
      height: 55,
      cells: [leaf('%1', 106, 55), leaf('%9', 106, 55, 107)],
    })

    it("opens a terminal per pane at the pane's own grid when a window is split", async () => {
      const { session, socket } = await attached()
      mountWindow(session, '@1')

      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%9', layout: split() }),
      })
      await flushPromises()

      const tab = session.tabs.value[0]
      expect(tab.panes.map((pane) => pane.paneId)).toEqual(['%1', '%9'])
      expect(tab.activePane).toBe('%9')
      const [left, right] = tab.panes
      expect(left.term.resize).toHaveBeenLastCalledWith(106, 55)
      expect(right.term.resize).toHaveBeenCalledWith(106, 55)

      socket.onmessage?.({ data: outputFrame('@1', '%1', 'left says') })
      socket.onmessage?.({ data: outputFrame('@1', '%9', 'right says') })
      const lastWrite = (term: { write: unknown }): string =>
        new TextDecoder().decode((term.write as ReturnType<typeof vi.fn>).mock.calls.at(-1)![0])
      expect(lastWrite(left.term)).toBe('left says')
      expect(lastWrite(right.term)).toBe('right says')
    })

    it('drops the pane a layout no longer holds and lands the keyboard on the new active pane', async () => {
      const { session, socket } = await attached()
      const host = mountWindow(session, '@1')
      // Focus starts inside the window to model the focus that pane removal displaces.
      const focused = document.createElement('input')
      host.append(focused)
      document.body.append(host)
      focused.focus()

      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%9', layout: split() }),
      })
      await flushPromises()
      const [left, , right] = xterm.FakeTerminal.instances
      // Opening the pane focused it; only the move counts from here.
      left.focus.mockClear()

      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', {
          name: 'agent',
          active: true,
          activePane: '%1',
          layout: leaf('%1', 213, 55),
        }),
      })
      await flushPromises()

      expect(right.dispose).toHaveBeenCalledTimes(1)
      expect(session.tabs.value[0].panes.map((pane) => pane.paneId)).toEqual(['%1'])
      expect(left.resize).toHaveBeenLastCalledWith(213, 55)
      expect(left.focus).toHaveBeenCalledTimes(1)
      host.remove()
    })

    it('leaves the keyboard alone when the active pane moves under a keyboard elsewhere', async () => {
      const { session, socket } = await attached()
      const host = mountWindow(session, '@1')
      document.body.append(host)
      const elsewhere = document.createElement('button')
      document.body.append(elsewhere)
      elsewhere.focus()

      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%9', layout: split() }),
      })
      await flushPromises()
      const newPaneHost = paneHost()
      host.append(newPaneHost)
      session.attachPane('%9', newPaneHost)
      await flushPromises()

      expect(session.tabs.value[0].activePane).toBe('%9')
      expect(xterm.FakeTerminal.instances[2].focus).not.toHaveBeenCalled()
      expect(document.activeElement).toBe(elsewhere)
      host.remove()
      elsewhere.remove()
    })

    it('draws a zoomed pane over the whole window and keeps the hidden one at its own size', async () => {
      const { session, socket } = await attached()
      mountWindow(session, '@1')
      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%9', layout: split() }),
      })
      await flushPromises()

      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', {
          name: 'agent',
          active: true,
          activePane: '%9',
          zoomed: true,
          layout: split(),
        }),
      })

      const [left, right] = session.tabs.value[0].panes
      expect(session.tabs.value[0].zoomed).toBe(true)
      expect(right.term.resize).toHaveBeenLastCalledWith(213, 55)
      expect(left.term.resize).toHaveBeenLastCalledWith(106, 55)
      expect(left.term.dispose).not.toHaveBeenCalled()
    })

    it('selects a clicked pane at once and tells tmux', async () => {
      const { client, session, socket } = await attached()
      mountWindow(session, '@1')
      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%9', layout: split() }),
      })
      await flushPromises()
      // Opening the pane focused it; only the click counts from here.
      xterm.FakeTerminal.instances[0].focus.mockClear()

      await session.selectPane('%1')

      const tab = session.tabs.value[0]
      expect(tab.activePane).toBe('%1')
      expect(xterm.FakeTerminal.instances[0].focus).toHaveBeenCalledTimes(1)
      expect(client.selectPane).toHaveBeenCalledWith('hive-abc', '%1')
      // Typing lands in the clicked pane before tmux has answered.
      xterm.FakeTerminal.instances[0].type('x')
      expect(Array.from(socket.sent.at(-1)!)).toEqual([0x10, 2, 0x25, 0x31, 0x78])

      client.selectPane.mockClear()
      await session.selectPane('%1')
      expect(client.selectPane).not.toHaveBeenCalled()
    })

    it('drives the pane verbs through the control plane against the active pane', async () => {
      const { client, session } = await attached()

      await session.splitPane('horizontal')
      expect(client.splitPane).toHaveBeenCalledWith('hive-abc', '%1', 'horizontal')
      await session.splitPane('vertical')
      expect(client.splitPane).toHaveBeenCalledWith('hive-abc', '%1', 'vertical')
      await session.zoomPane()
      expect(client.zoomPane).toHaveBeenCalledWith('hive-abc', '%1')
      await session.focusPane('down')
      expect(client.selectPane).toHaveBeenCalledWith('hive-abc', '%1', 'down')
      await session.closePane()
      expect(client.closePane).toHaveBeenCalledWith('hive-abc', '%1')
      await session.closePane('%2')
      expect(client.closePane).toHaveBeenCalledWith('hive-abc', '%2')
    })

    it('sends only the latest size of a divider drag while one is in flight', async () => {
      const { client, session } = await attached()
      let release: (() => void) | undefined
      client.resizePane.mockImplementationOnce(
        () =>
          new Promise<void>((resolve) => {
            release = resolve
          }),
      )

      const first = session.resizePane('%1', { width: 50 })
      void session.resizePane('%1', { width: 51 })
      void session.resizePane('%1', { width: 52 })
      expect(client.resizePane).toHaveBeenCalledTimes(1)

      release?.()
      await first
      await flushPromises()
      expect(client.resizePane).toHaveBeenCalledTimes(2)
      expect(client.resizePane).toHaveBeenLastCalledWith('hive-abc', '%1', { width: 52 })
    })

    it('measures the cell off the first pane it opens', async () => {
      const { session } = await attached()
      expect(session.cell.value).toBeNull()
      mountWindow(session, '@1')
      expect(session.cell.value).toEqual({ width: 10, height: 25 })
    })

    // A hit count is only true of the buffer it was counted in, so the bar
    // follows the active pane the way it follows the active window.
    it('re-runs the search against the pane tmux makes active', async () => {
      const { session, socket } = await attached()
      mountWindow(session, '@1')
      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%1', layout: split() }),
      })
      await flushPromises()
      session.openSearch()
      session.setSearchQuery('panic')
      const [left, , right] = xterm.FakeSearchAddon.instances
      left.clearDecorations.mockClear()
      expect(right.calls).toEqual([])

      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%9', layout: split() }),
      })

      expect(left.clearDecorations).toHaveBeenCalled()
      expect(right.calls.at(-1)).toMatchObject({ mode: 'next', term: 'panic' })
    })

    it('re-runs the search against a clicked pane', async () => {
      const { session, socket } = await attached()
      mountWindow(session, '@1')
      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%1', layout: split() }),
      })
      await flushPromises()
      session.openSearch()
      session.setSearchQuery('panic')
      const [left, , right] = xterm.FakeSearchAddon.instances
      left.clearDecorations.mockClear()

      await session.selectPane('%9')

      expect(left.clearDecorations).toHaveBeenCalled()
      expect(right.calls.at(-1)).toMatchObject({ mode: 'next', term: 'panic' })
    })

    it('leaves a closed find bar alone when the active pane changes', async () => {
      const { session, socket } = await attached()
      mountWindow(session, '@1')
      socket.onmessage?.({
        data: windowFrame('layout-changed', '@1', { name: 'agent', active: true, activePane: '%1', layout: split() }),
      })
      await flushPromises()
      const [left, , right] = xterm.FakeSearchAddon.instances
      left.clearDecorations.mockClear()

      await session.selectPane('%9')

      expect(left.clearDecorations).not.toHaveBeenCalled()
      expect(right.calls).toEqual([])
    })
  })
})
