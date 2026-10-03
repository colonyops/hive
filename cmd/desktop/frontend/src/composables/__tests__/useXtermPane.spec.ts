import { flushPromises } from '@vue/test-utils'
import { effectScope, ref, type EffectScope } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createTerminal, useXtermPane } from '../useXtermPane'
import { setTheme } from '../useTheme'
import { useTerminalFont } from '../../stores/useTerminalFont'

const xterm = vi.hoisted(() => {
  class FakeTerminal {
    static instances: FakeTerminal[] = []
    options: Record<string, unknown>
    open = vi.fn()
    focus = vi.fn()
    dispose = vi.fn()
    loadAddon = vi.fn()
    keyHandler: ((event: KeyboardEvent) => boolean) | null = null

    constructor(options: Record<string, unknown> = {}) {
      this.options = { ...options }
      FakeTerminal.instances.push(this)
    }

    attachCustomKeyEventHandler(handler: (event: KeyboardEvent) => boolean) {
      this.keyHandler = handler
    }
  }

  class FakeFitAddon {
    fit = vi.fn()
    proposeDimensions = vi.fn(() => ({ cols: 132, rows: 43 }))
  }

  class FakeWebLinksAddon {
    constructor(public handler: (event: MouseEvent, uri: string) => void) {}
  }

  return { FakeTerminal, FakeFitAddon, FakeWebLinksAddon }
})

const mocks = vi.hoisted(() => ({
  OpenURL: vi.fn(() => Promise.resolve()),
  claimAtlasRenderer: vi.fn(),
  loadTerminalFaces: vi.fn(() => Promise.resolve()),
}))

vi.mock('@xterm/xterm', () => ({ Terminal: xterm.FakeTerminal }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: xterm.FakeFitAddon }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: xterm.FakeWebLinksAddon }))
vi.mock('@wailsio/runtime', () => ({ Browser: { OpenURL: mocks.OpenURL }, Events: { On: vi.fn(() => vi.fn()) } }))
vi.mock('../../lib/terminalRenderer', () => ({ claimAtlasRenderer: mocks.claimAtlasRenderer }))
vi.mock('../../lib/terminalFaces', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/terminalFaces')>()),
  loadTerminalFaces: mocks.loadTerminalFaces,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn().mockResolvedValue({ theme: '', terminalFontSizePx: 13, terminalFontFamily: '' }),
  SetTheme: vi.fn(),
  SetTerminalFontSize: vi.fn(),
  SetTerminalFontFamily: vi.fn(),
  SetTerminalFontWeights: vi.fn(),
  SetTerminalLineHeight: vi.fn(),
  SetTerminalLetterSpacing: vi.fn(),
}))

class FakeSocket {
  readyState: number = WebSocket.OPEN
  sent: unknown[] = []
  closed = false
  onmessage: unknown = () => {}
  onerror: unknown = () => {}
  onclose: unknown = () => {}
  send(frame: unknown) {
    this.sent.push(frame)
  }
  close() {
    this.closed = true
  }
}

function boxedHost(width = 800, height = 600): HTMLElement {
  const host = document.createElement('div')
  Object.defineProperty(host, 'clientWidth', { value: width })
  Object.defineProperty(host, 'clientHeight', { value: height })
  return host
}

let scope: EffectScope

function setup(host: HTMLElement | null, onResize = vi.fn()) {
  scope = effectScope()
  const pane = scope.run(() => useXtermPane(ref(host), onResize))!
  return { pane, onResize }
}

describe('useXtermPane', () => {
  beforeEach(() => {
    xterm.FakeTerminal.instances = []
    vi.clearAllMocks()
  })

  afterEach(() => {
    scope?.stop()
    vi.useRealTimers()
  })

  it('creates a terminal at the configured font that opens links outside the webview', () => {
    useTerminalFont().setFontFamily('Menlo')
    const term = createTerminal({ allowProposedApi: true }) as unknown as InstanceType<typeof xterm.FakeTerminal>

    expect(term.options.fontFamily).toContain('Menlo')
    expect(term.options.scrollback).toBe(5000)
    expect(term.options.allowProposedApi).toBe(true)
    ;(term.options.linkHandler as { activate: (e: MouseEvent, uri: string) => void }).activate(
      new MouseEvent('click'),
      'https://example.test/osc8',
    )
    const links = term.loadAddon.mock.calls[0][0] as InstanceType<typeof xterm.FakeWebLinksAddon>
    links.handler(new MouseEvent('click'), 'https://example.test/bare')

    expect(mocks.OpenURL).toHaveBeenCalledWith('https://example.test/osc8')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://example.test/bare')
  })

  it('builds nothing without a host', () => {
    const { pane } = setup(null)
    expect(pane.build()).toBeNull()
    expect(pane.term.value).toBeNull()
  })

  it('opens the pane on its host and claims a renderer after open', () => {
    const host = boxedHost()
    const { pane } = setup(host)
    const created = pane.build() as unknown as InstanceType<typeof xterm.FakeTerminal>

    expect(created.open).toHaveBeenCalledWith(host)
    expect(mocks.claimAtlasRenderer).toHaveBeenCalledWith(created, expect.any(Function), expect.any(Function))
    expect(created.open.mock.invocationCallOrder[0]).toBeLessThan(mocks.claimAtlasRenderer.mock.invocationCallOrder[0])
    expect(pane.term.value).toBe(created)
  })

  it('measures the host grid, and nothing for a host with no box', () => {
    const { pane } = setup(boxedHost())
    pane.build()
    expect(pane.measure()).toEqual({ cols: 132, rows: 43 })

    const empty = setup(boxedHost(0, 0)).pane
    empty.build()
    expect(empty.measure()).toBeNull()
  })

  it('retries a renderer claim that did not survive, and only then', () => {
    const { pane } = setup(boxedHost())
    pane.build()
    const record = mocks.claimAtlasRenderer.mock.calls[0][2] as (rendered: boolean) => void

    record(true)
    pane.claimRenderer()
    expect(mocks.claimAtlasRenderer).toHaveBeenCalledTimes(1)

    record(false)
    pane.claimRenderer()
    expect(mocks.claimAtlasRenderer).toHaveBeenCalledTimes(2)
  })

  it('sends frames only while the attached socket is open', () => {
    const { pane } = setup(boxedHost())
    pane.build()
    const socket = new FakeSocket()
    const frame = new Uint8Array([1])

    pane.send([frame])
    pane.attach(socket as unknown as WebSocket)
    pane.send([frame])
    socket.readyState = WebSocket.CLOSED
    pane.send([frame])

    expect(socket.sent).toEqual([frame])
    expect(pane.socket()).toBe(socket)
  })

  it('tears down the stream, then the tracked disposers, then the terminal', () => {
    const { pane } = setup(boxedHost())
    const created = pane.build() as unknown as InstanceType<typeof xterm.FakeTerminal>
    const tracked = { dispose: vi.fn() }
    pane.track(tracked)
    const socket = new FakeSocket()
    pane.attach(socket as unknown as WebSocket)

    pane.teardown()

    expect(socket.closed).toBe(true)
    expect(socket.onmessage).toBeNull()
    expect(socket.onclose).toBeNull()
    expect(tracked.dispose.mock.invocationCallOrder[0]).toBeLessThan(created.dispose.mock.invocationCallOrder[0])
    expect(pane.term.value).toBeNull()
    expect(pane.socket()).toBeNull()
  })

  it('applies a new font once its faces load and then asks for a debounced resize', async () => {
    vi.useFakeTimers()
    const { pane, onResize } = setup(boxedHost())
    const created = pane.build() as unknown as InstanceType<typeof xterm.FakeTerminal>

    useTerminalFont().setFontFamily('Iosevka')
    await flushPromises()

    expect(mocks.loadTerminalFaces).toHaveBeenCalled()
    expect(created.options.fontFamily).toContain('Iosevka')
    expect(onResize).not.toHaveBeenCalled()
    vi.advanceTimersByTime(80)
    expect(onResize).toHaveBeenCalledTimes(1)
  })

  it('repaints the live pane when the theme changes', async () => {
    const { pane } = setup(boxedHost())
    const created = pane.build() as unknown as InstanceType<typeof xterm.FakeTerminal>
    const before = created.options.theme

    setTheme('light')
    await flushPromises()

    expect(created.options.theme).not.toBe(before)
  })
})
