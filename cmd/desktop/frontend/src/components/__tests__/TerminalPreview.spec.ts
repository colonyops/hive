import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalPreview from '../settings/TerminalPreview.vue'
import {
  defaultTerminalFontWeight,
  defaultTerminalFontWeightBold,
  defaultTerminalLetterSpacing,
  defaultTerminalLineHeight,
  defaultTerminalFontSizePx,
  useTerminalFont,
} from '../../stores/useTerminalFont'
import { terminalFontStack } from '../../lib/terminalFaces'

const xterm = vi.hoisted(() => {
  class FakeTerminal {
    static instances: FakeTerminal[] = []
    options: Record<string, unknown> = {}
    // The order disposal actually happened in, which is the invariant that
    // matters: an addon disposed after the core cannot restore a renderer.
    static disposals: string[] = []
    write = vi.fn()
    open = vi.fn()
    loadAddon = vi.fn()
    resize = vi.fn()
    dispose = vi.fn(() => {
      FakeTerminal.disposals.push('terminal')
    })

    constructor(options: Record<string, unknown> = {}) {
      this.options = { ...options }
      FakeTerminal.instances.push(this)
    }
  }

  class FakeAddon {
    static instances: FakeAddon[] = []
    dispose = vi.fn(() => {
      FakeTerminal.disposals.push('addon')
    })
    activate = vi.fn()
    onContextLoss = vi.fn(() => ({ dispose: vi.fn() }))
    constructor() {
      FakeAddon.instances.push(this)
    }
  }

  return { FakeTerminal, FakeAddon }
})

vi.mock('@xterm/xterm', () => ({ Terminal: xterm.FakeTerminal }))
vi.mock('@xterm/addon-webgl', () => ({ WebglAddon: xterm.FakeAddon }))
vi.mock('@xterm/addon-canvas', () => ({ CanvasAddon: xterm.FakeAddon }))

const mocks = vi.hoisted(() => ({ loadTerminalFaces: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn().mockResolvedValue({
    theme: '',
    terminalFontSizePx: 13,
    terminalFontFamily: '',
    terminalFontWeight: 0,
    terminalFontWeightBold: 0,
    terminalLineHeight: 0,
    terminalLetterSpacing: 0,
    terminalShowWindows: true,
    terminalPoolSize: 3,
  }),
  Fonts: vi.fn().mockResolvedValue({ all: [], monospace: [] }),
  SetTheme: vi.fn(),
  SetTerminalFontSize: vi.fn(),
  SetTerminalFontFamily: vi.fn(),
  SetTerminalFontWeights: vi.fn(),
  SetTerminalLineHeight: vi.fn(),
  SetTerminalLetterSpacing: vi.fn(),
}))
vi.mock('../../lib/terminalFaces', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/terminalFaces')>()),
  loadTerminalFaces: mocks.loadTerminalFaces,
}))

async function preview() {
  const wrapper = mount(TerminalPreview, { attachTo: document.body })
  await flushPromises()
  return wrapper
}

describe('TerminalPreview', () => {
  beforeEach(() => {
    xterm.FakeTerminal.instances = []
    xterm.FakeTerminal.disposals = []
    xterm.FakeAddon.instances = []
    mocks.loadTerminalFaces.mockClear()
    mocks.loadTerminalFaces.mockResolvedValue(undefined)
  })

  it('opens on the same typography the panes render with', async () => {
    await preview()

    const [term] = xterm.FakeTerminal.instances
    expect(term.options).toMatchObject({
      fontFamily: terminalFontStack(''),
      fontSize: defaultTerminalFontSizePx,
      fontWeight: defaultTerminalFontWeight,
      fontWeightBold: defaultTerminalFontWeightBold,
      lineHeight: defaultTerminalLineHeight,
      letterSpacing: defaultTerminalLetterSpacing,
    })
  })

  // The preview only means anything if it rasterises the way a pane does, and
  // box drawing is the difference: the DOM renderer takes it from the font
  // rather than stroking it to the cell. ADR terminal-atlas-renderer.
  it('claims an atlas renderer, after opening rather than before', async () => {
    await preview()

    const [term] = xterm.FakeTerminal.instances
    expect(xterm.FakeAddon.instances).toHaveLength(1)
    expect(term.open.mock.invocationCallOrder[0]).toBeLessThan(term.loadAddon.mock.invocationCallOrder[0])
  })

  // xterm measures its cell on open() and never re-measures, and the atlas
  // caches what was resident, so a face arriving late stays wrong. ADR terminal-atlas-renderer.
  it('makes the faces resident before it opens', async () => {
    await preview()

    const [term] = xterm.FakeTerminal.instances
    expect(mocks.loadTerminalFaces).toHaveBeenCalledWith(
      '',
      defaultTerminalFontSizePx,
      defaultTerminalFontWeight,
      defaultTerminalFontWeightBold,
    )
    expect(mocks.loadTerminalFaces.mock.invocationCallOrder[0]).toBeLessThan(term.open.mock.invocationCallOrder[0])
  })

  it('re-applies every typography setting without reopening', async () => {
    await preview()

    const font = useTerminalFont()
    font.setLineHeight(1.5)
    font.setLetterSpacing(2)
    font.setFontSize(16)
    await flushPromises()

    expect(xterm.FakeTerminal.instances).toHaveLength(1)
    expect(xterm.FakeTerminal.instances[0].options).toMatchObject({
      lineHeight: 1.5,
      letterSpacing: 2,
      fontSize: 16,
    })
  })

  it('disposes the renderer addon ahead of the terminal', async () => {
    const wrapper = await preview()

    wrapper.unmount()

    expect(xterm.FakeTerminal.disposals).toEqual(['addon', 'terminal'])
  })

  // Nothing to type into, and nothing worth a tab stop on the way through the
  // settings pane.
  it('takes its input textarea out of the tab order', async () => {
    const wrapper = mount(TerminalPreview, { attachTo: document.body })
    const host = wrapper.get('[data-testid="settings-terminal-preview-pane"]').element
    const textarea = document.createElement('textarea')
    host.appendChild(textarea)
    await flushPromises()

    expect(textarea.getAttribute('tabindex')).toBe('-1')
  })
})
