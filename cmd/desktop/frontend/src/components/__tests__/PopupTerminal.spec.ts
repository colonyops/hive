import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PopupTerminal from '../PopupTerminal.vue'
import { resetPopupTerminalForTests, usePopupTerminal } from '../../composables/usePopupTerminal'
import {
  defaultTerminalFontWeight,
  defaultTerminalFontWeightBold,
  setTerminalFontFamily,
  setTerminalFontWeight,
} from '../../composables/useTerminalFont'
import { TERMINAL_FONT, terminalFontStack } from '../../lib/terminalFaces'

const xterm = vi.hoisted(() => {
  class FakeTerminal {
    static instances: FakeTerminal[] = []
    cols = 80
    rows = 24
    options: Record<string, unknown> = {}
    write = vi.fn()
    paste = vi.fn((text: string) => this.type(text))
    // Focusing an element inside a display:none subtree silently does nothing,
    // so what matters is not that focus() was called but that the pane was on
    // screen when it was.
    focusedWithPaneDisplay: string | null = null
    focus = vi.fn(() => {
      const pane = document.querySelector<HTMLElement>('[data-testid="popup-terminal-pane"]')
      this.focusedWithPaneDisplay = pane ? pane.style.display : null
    })
    open = vi.fn()
    loadAddon = vi.fn()
    dispose = vi.fn()
    resize = vi.fn()
    private dataHandlers: ((data: string) => void)[] = []
    private resizeHandlers: ((size: { cols: number; rows: number }) => void)[] = []

    constructor(options: Record<string, unknown> = {}) {
      this.options = { ...options }
      FakeTerminal.instances.push(this)
    }

    onData(handler: (data: string) => void) {
      this.dataHandlers.push(handler)
      return { dispose: vi.fn() }
    }

    onResize(handler: (size: { cols: number; rows: number }) => void) {
      this.resizeHandlers.push(handler)
      return { dispose: vi.fn() }
    }

    type(data: string): void {
      for (const handler of this.dataHandlers) handler(data)
    }

    reflow(cols: number, rows: number): void {
      for (const handler of this.resizeHandlers) handler({ cols, rows })
    }
  }

  class FakeAddon {
    dispose = vi.fn()
    activate = vi.fn()
    fit = vi.fn()
    proposeDimensions = vi.fn(() => ({ cols: 132, rows: 43 }))
    onContextLoss = vi.fn(() => ({ dispose: vi.fn() }))
  }

  return { FakeTerminal, FakeAddon }
})

vi.mock('@xterm/xterm', () => ({ Terminal: xterm.FakeTerminal }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: xterm.FakeAddon }))
vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: vi.fn().mockResolvedValue(undefined) },
  Events: { On: vi.fn(() => vi.fn()) },
}))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: xterm.FakeAddon }))
vi.mock('@xterm/addon-webgl', () => ({ WebglAddon: xterm.FakeAddon }))
vi.mock('@xterm/addon-canvas', () => ({ CanvasAddon: xterm.FakeAddon }))

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getPopupTerminalEndpoint: vi.fn(),
  createPopupTerminalClient: vi.fn(),
  loadTerminalFaces: vi.fn(),
}))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice',
  () => ({
    Available: mocks.Available,
  }),
)
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn().mockResolvedValue({
    theme: '',
    terminalFontSizePx: 13,
    terminalFontFamily: '',
    terminalFontWeight: 0,
    terminalFontWeightBold: 0,
    terminalShowWindows: true,
    terminalPoolSize: 3,
  }),
  Fonts: vi.fn().mockResolvedValue({ all: [], monospace: [] }),
  SetTheme: vi.fn(),
  SetTerminalFontSize: vi.fn(),
  SetTerminalFontFamily: vi.fn(),
  SetTerminalFontWeights: vi.fn(),
}))
vi.mock('../../lib/terminalFaces', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/terminalFaces')>()),
  loadTerminalFaces: mocks.loadTerminalFaces,
}))
vi.mock('../../lib/popupTerminalClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/popupTerminalClient')>()),
  getPopupTerminalEndpoint: mocks.getPopupTerminalEndpoint,
  createPopupTerminalClient: mocks.createPopupTerminalClient,
}))

class FakeSocket {
  static instances: FakeSocket[] = []
  static OPEN = 1
  readyState = 1
  binaryType = 'blob'
  sent: Uint8Array[] = []
  closed = false
  onmessage: ((event: { data: Uint8Array }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor() {
    FakeSocket.instances.push(this)
  }
  send(frame: Uint8Array): void {
    this.sent.push(frame)
  }
  close(): void {
    this.closed = true
  }
}

class FakeResizeObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  constructor(_callback: () => void) {}
}

const encoder = new TextEncoder()

function fakeClient() {
  return {
    open: vi.fn().mockResolvedValue({ id: 't1', title: 'zsh', dir: '/tmp/checkout', command: '', cols: 80, rows: 24 }),
    close: vi.fn().mockResolvedValue({ closed: true }),
    list: vi.fn().mockResolvedValue([]),
    resize: vi.fn().mockResolvedValue(undefined),
    openStream: vi.fn(() => new FakeSocket()),
  }
}

// The panel teleports to the body, so it is queried there rather than through
// the wrapper.
function el<T extends HTMLElement>(testid: string): T {
  const element = document.querySelector<T>(`[data-testid="${testid}"]`)
  if (!element) throw new Error(`Missing ${testid}`)
  return element
}

// jsdom lays nothing out, so a pane only has a box when one is declared. Without
// this every pane measures to nothing, which is its own case below.
function givePaneABox(): void {
  for (const prop of ['clientWidth', 'clientHeight'] as const) {
    Object.defineProperty(HTMLElement.prototype, prop, { configurable: true, value: 800 })
  }
}

function takePaneBoxAway(): void {
  for (const prop of ['clientWidth', 'clientHeight'] as const) {
    delete (HTMLElement.prototype as Partial<HTMLElement>)[prop]
  }
}

// The panel state is a module singleton, so a panel left mounted by an earlier
// test would open a terminal of its own on the next show().
let mounted: ReturnType<typeof mount> | null = null

async function mountPanel(client = fakeClient()) {
  mocks.createPopupTerminalClient.mockReturnValue(client)
  const wrapper = mount(PopupTerminal, { attachTo: document.body })
  mounted = wrapper
  await flushPromises()
  return { wrapper, client }
}

describe('PopupTerminal', () => {
  afterEach(() => {
    mounted?.unmount()
    mounted = null
    takePaneBoxAway()
  })

  beforeEach(() => {
    vi.useRealTimers()
    resetPopupTerminalForTests()
    xterm.FakeTerminal.instances = []
    FakeSocket.instances = []
    localStorage.clear()
    document.body.innerHTML = ''
    globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
    globalThis.ResizeObserver = FakeResizeObserver as unknown as typeof ResizeObserver
    mocks.loadTerminalFaces.mockResolvedValue(undefined)
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getPopupTerminalEndpoint.mockResolvedValue({
      httpBaseURL: 'http://127.0.0.1:58006',
      wsURL: 'ws://127.0.0.1:58006/api/terminal/pty/stream',
      token: 'tok',
    })
  })

  // App.vue mounts the panel lazily *from* the same action that shows it, so
  // the panel comes up with visible already true and has no change to react to.
  // Mounting after the show is the order that matters; mounting before it is
  // the order that only happens in a test.
  it('opens a terminal when it is mounted already visible', async () => {
    const popup = usePopupTerminal()
    popup.show({ sessionSlug: 'hive-abc' })

    const { client } = await mountPanel()
    await flushPromises()

    expect(client.open).toHaveBeenCalledWith({ sessionSlug: 'hive-abc' })
    expect(xterm.FakeTerminal.instances).toHaveLength(1)
  })

  it('opens a terminal against the context it was asked for', async () => {
    const { client } = await mountPanel()

    usePopupTerminal().show({ sessionSlug: 'hive-abc' })
    await flushPromises()

    expect(client.open).toHaveBeenCalledWith({ sessionSlug: 'hive-abc' })
    expect(client.openStream).toHaveBeenCalledWith('t1')
    expect(xterm.FakeTerminal.instances).toHaveLength(1)
  })

  // One pop-up is open at a time (ADR ephemeral-popup-terminals), so asking for lazygit while a
  // shell is up is a request to see lazygit — not to be handed the shell back.
  it('replaces the live terminal when a different launch is asked for', async () => {
    const { client } = await mountPanel()
    const popup = usePopupTerminal()

    popup.show({ sessionSlug: 'hive-abc' })
    await flushPromises()
    expect(client.open).toHaveBeenCalledTimes(1)

    popup.show({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    await flushPromises()

    expect(client.open).toHaveBeenCalledTimes(2)
    expect(client.open).toHaveBeenLastCalledWith({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    expect(client.close).toHaveBeenCalledWith('t1')
  })

  // The shell's own behaviour is unchanged by launchers existing: the same
  // request twice is the same terminal, hidden and brought back.
  it('returns to the live terminal when the same launch is asked for again', async () => {
    const { client } = await mountPanel()
    const popup = usePopupTerminal()

    popup.show({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    await flushPromises()
    popup.hide()
    await flushPromises()
    popup.show({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    await flushPromises()

    expect(client.open).toHaveBeenCalledTimes(1)
    expect(client.close).not.toHaveBeenCalled()
  })

  // Popping up a terminal you cannot type into is not popping up a terminal.
  it('focuses the pane once it is actually on screen', async () => {
    await mountPanel()
    usePopupTerminal().show()
    await flushPromises()

    const created = xterm.FakeTerminal.instances[0]
    expect(created.focus).toHaveBeenCalled()
    expect(created.focusedWithPaneDisplay).not.toBe('none')
  })

  // Agent TUIs draw Nerd Font glyphs no system font covers, and xterm measures
  // its cell when a Terminal opens and never re-measures — so a pane built
  // ahead of the face renders tofu for the rest of its life.
  it('waits for the terminal face before building the pane', async () => {
    let releaseFaces = (): void => {}
    mocks.loadTerminalFaces.mockReturnValue(
      new Promise<void>((resolve) => {
        releaseFaces = resolve
      }),
    )

    await mountPanel()
    usePopupTerminal().show()
    await flushPromises()

    expect(mocks.loadTerminalFaces).toHaveBeenCalled()
    expect(xterm.FakeTerminal.instances).toHaveLength(0)

    releaseFaces()
    await flushPromises()

    expect(xterm.FakeTerminal.instances).toHaveLength(1)
    expect(xterm.FakeTerminal.instances[0].options.fontFamily).toContain('Nerd Font')
  })

  // A launcher opens through the same path a bare shell does, so it has to
  // carry the same typography — a pop-up that ignored the font settings would
  // be the one terminal surface that does.
  it('opens a launcher pop-up with the configured font', async () => {
    await mountPanel()
    setTerminalFontFamily('Menlo')
    setTerminalFontWeight(400)
    await flushPromises()

    usePopupTerminal().show({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    await flushPromises()

    const opened = xterm.FakeTerminal.instances.at(-1)!
    expect(opened.options.fontFamily).toBe(terminalFontStack('Menlo'))
    expect(opened.options.fontWeight).toBe(400)
    expect(opened.options.fontWeightBold).toBe(defaultTerminalFontWeightBold)
    // The faces have to be resident before the Terminal is constructed: xterm
    // measures its cell on open and never re-measures (ADR terminal-atlas-renderer).
    expect(mocks.loadTerminalFaces).toHaveBeenCalledWith(
      'Menlo',
      expect.any(Number),
      400,
      defaultTerminalFontWeightBold,
    )

    setTerminalFontFamily(TERMINAL_FONT)
    setTerminalFontWeight(defaultTerminalFontWeight)
    await flushPromises()
  })

  it('applies a font change to the pop-up already on screen', async () => {
    await mountPanel()
    usePopupTerminal().show()
    await flushPromises()
    const live = xterm.FakeTerminal.instances.at(-1)!

    setTerminalFontWeight(700)
    await flushPromises()

    expect(live.options.fontWeight).toBe(700)
    // Reopening is what would lose the scrollback, so the change has to land on
    // the terminal that is already up.
    expect(xterm.FakeTerminal.instances.at(-1)).toBe(live)

    setTerminalFontWeight(defaultTerminalFontWeight)
    await flushPromises()
  })

  it('writes output to the pane and keystrokes to the socket', async () => {
    await mountPanel()
    usePopupTerminal().show()
    await flushPromises()

    const socket = FakeSocket.instances[0]
    socket.onmessage?.({ data: new Uint8Array([0x00, ...encoder.encode('hello')]) })
    expect(xterm.FakeTerminal.instances[0].write).toHaveBeenCalledWith(encoder.encode('hello'))

    xterm.FakeTerminal.instances[0].type('ls\n')
    expect(socket.sent[0][0]).toBe(0x10)
    expect(new TextDecoder().decode(socket.sent[0].subarray(1))).toBe('ls\n')
  })

  // Hiding is a view change: the shell has to survive it, or the shortcut that
  // dismisses the panel would silently kill whatever was running.
  it('keeps the shell running when the panel is hidden', async () => {
    const { client } = await mountPanel()
    const popup = usePopupTerminal()
    popup.show()
    await flushPromises()

    el('popup-terminal-hide').click()
    await flushPromises()

    expect(client.close).not.toHaveBeenCalled()
    expect(FakeSocket.instances[0].closed).toBe(false)

    popup.show()
    await flushPromises()
    expect(client.open).toHaveBeenCalledTimes(1)
    // Coming back to a running shell has to be typeable too, not just opening
    // a fresh one.
    const created = xterm.FakeTerminal.instances[0]
    expect(created.focus).toHaveBeenCalledTimes(2)
    expect(created.focusedWithPaneDisplay).not.toBe('none')
  })

  it('ends the terminal on purpose from the header', async () => {
    const { client } = await mountPanel()
    const popup = usePopupTerminal()
    popup.show()
    await flushPromises()

    el('popup-terminal-end').click()
    await flushPromises()

    expect(client.close).toHaveBeenCalledWith('t1')
    expect(FakeSocket.instances[0].closed).toBe(true)
    expect(popup.visible.value).toBe(false)
  })

  // Typing `exit` is how a pop-up is dismissed, so the panel goes with the
  // shell rather than leaving a dead pane between the user and the app.
  it('closes the panel when the shell exits', async () => {
    const { client } = await mountPanel()
    const popup = usePopupTerminal()
    popup.show()
    await flushPromises()

    FakeSocket.instances[0].onmessage?.({
      data: new Uint8Array([0x01, ...encoder.encode(JSON.stringify({ reason: 'exited with status 0' }))]),
    })
    await flushPromises()

    expect(popup.visible.value).toBe(false)
    // A process that exited took its terminal with it, so there is nothing to
    // close and nothing to reattach to.
    expect(client.close).not.toHaveBeenCalled()

    popup.show()
    await flushPromises()
    expect(client.open).toHaveBeenCalledTimes(2)
  })

  // A dropped stream is not an exit: the shell may still be alive, and
  // vanishing without saying so would read as the terminal closing itself.
  it('keeps the panel up when the stream drops', async () => {
    const popup = usePopupTerminal()
    await mountPanel()
    popup.show()
    await flushPromises()

    FakeSocket.instances[0].onerror?.()
    await flushPromises()

    expect(popup.visible.value).toBe(true)
    expect(el('popup-terminal-ended').textContent).toContain('connection dropped')
    expect(el('popup-terminal-pane').style.display).not.toBe('none')
  })

  // A stream that dropped for a reason other than the process exiting would
  // otherwise leave a shell running with nothing able to reach it again.
  it('closes the terminal it replaces rather than abandoning it', async () => {
    const { client } = await mountPanel()
    usePopupTerminal().show()
    await flushPromises()

    FakeSocket.instances[0].onerror?.()
    await flushPromises()

    el('popup-terminal-new').click()
    await flushPromises()

    expect(client.close).toHaveBeenCalledWith('t1')
    expect(client.open).toHaveBeenCalledTimes(2)
  })

  // The pop-up is reached for constantly, so where it lands must not depend on
  // where it was left or on what the window was the first time it opened.
  it('opens centred at 85% of the window', async () => {
    await mountPanel()
    usePopupTerminal().show()
    await flushPromises()

    const panel = el('popup-terminal')
    const width = Number.parseInt(panel.style.width, 10)
    const height = Number.parseInt(panel.style.height, 10)
    const x = Number.parseInt(panel.style.left, 10)
    const y = Number.parseInt(panel.style.top, 10)

    expect(width).toBe(Math.round(window.innerWidth * 0.85))
    expect(height).toBe(Math.round(window.innerHeight * 0.85))
    expect(Math.abs(x - (window.innerWidth - width - x))).toBeLessThanOrEqual(1)
    expect(Math.abs(y - (window.innerHeight - height - y))).toBeLessThanOrEqual(1)
  })

  // The box is derived from the window and nothing else, so resizing the app
  // window resizes the pop-up rather than leaving it at whatever it opened as.
  it('follows the window when it is resized', async () => {
    await mountPanel()
    usePopupTerminal().show()
    await flushPromises()

    window.innerWidth = 1600
    window.innerHeight = 1000
    window.dispatchEvent(new Event('resize'))
    await flushPromises()

    const panel = el('popup-terminal')
    expect(Number.parseInt(panel.style.width, 10)).toBe(Math.round(1600 * 0.85))
    expect(Number.parseInt(panel.style.height, 10)).toBe(Math.round(1000 * 0.85))
  })

  // The pop-up is reached for mid-task, so dismissing it has to put the user
  // back where they were rather than on the document body.
  it('returns focus to whatever had it before', async () => {
    const caller = document.createElement('input')
    document.body.appendChild(caller)
    caller.focus()

    await mountPanel()
    const popup = usePopupTerminal()
    popup.show()
    await flushPromises()

    popup.hide()
    await flushPromises()

    expect(document.activeElement).toBe(caller)
  })

  it('renders the reason instead of a shell when PTYs are unavailable', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'Terminal features are off.' })
    const { client } = await mountPanel()

    usePopupTerminal().show()
    await flushPromises()

    expect(el('popup-terminal-unavailable').textContent).toBe('Terminal features are off.')
    expect(el('popup-terminal-pane').style.display).toBe('none')
    expect(client.open).not.toHaveBeenCalled()
  })

  // A PTY sized only after the process started paints one full screen at the
  // placeholder grid and reflows on SIGWINCH — which is a TUI popping up small
  // and snapping wider. The size has to ride the launch.
  it('spawns the process at the grid the pane measured', async () => {
    givePaneABox()
    const { client } = await mountPanel()

    usePopupTerminal().show({ sessionSlug: 'hive-abc' })
    await flushPromises()

    expect(client.open).toHaveBeenCalledWith({ sessionSlug: 'hive-abc', cols: 132, rows: 43 })
    // The pane opens on the same grid, so the first frame is the right one and
    // there is no resize to send once the process is running.
    expect(xterm.FakeTerminal.instances[0].resize).toHaveBeenCalledWith(132, 43)
    expect(client.resize).not.toHaveBeenCalled()
  })

  // proposeDimensions on a host with no box answers a bogus tiny grid rather
  // than failing, so an unmeasurable pane leaves the size to the server instead
  // of spawning at one that is wrong on purpose.
  it('leaves the size to the server when the pane cannot be measured', async () => {
    const { client } = await mountPanel()

    usePopupTerminal().show({ sessionSlug: 'hive-abc' })
    await flushPromises()

    expect(client.open).toHaveBeenCalledWith({ sessionSlug: 'hive-abc' })
    expect(xterm.FakeTerminal.instances[0].resize).not.toHaveBeenCalled()
  })

  // The server applies whatever size it is told, so what xterm measured is what
  // the process gets — there is nothing to reconcile afterwards.
  it('tells the process the size the pane measured', async () => {
    const { client } = await mountPanel()
    usePopupTerminal().show()
    await flushPromises()

    xterm.FakeTerminal.instances[0].reflow(132, 43)
    await flushPromises()

    expect(client.resize).toHaveBeenCalledWith('t1', 132, 43)
  })
})
