import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../defineStore'
import { documentPx, nextScale, terminalPx, useSurfaceScale } from '../useSurfaceScale'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetInterfaceScale: vi.fn(),
  SetDocumentScale: vi.fn(),
  SetCodeScale: vi.fn(),
  SetTerminalScale: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => mocks)

describe('scale arithmetic', () => {
  it.each([
    ['interface', 100, 1, 110],
    ['interface', 100, -1, 90],
    ['interface', 130, 1, 150],
    ['interface', 150, 1, 150],
    ['interface', 75, -1, 75],
    ['terminal', 75, -1, 60],
    ['terminal', 400, 1, 500],
    ['code', 175, 1, 200],
    ['terminal', 115, 1, 120],
    ['terminal', 115, -1, 110],
    ['terminal', 62, 1, 75],
    ['terminal', 492, 1, 500],
  ] as const)('steps %s from %i by %i to %i', (surface, from, direction, to) => {
    expect(nextScale(surface, from, direction)).toBe(to)
  })

  it('resolves the terminal to whole pixels that every old size round-trips through', () => {
    expect(terminalPx(100)).toBe(13)
    expect(terminalPx(110)).toBe(14)
    expect(terminalPx(120)).toBe(16)
    for (let px = 8; px <= 64; px++) expect(terminalPx(Math.round((px * 100) / 13))).toBe(px)
  })

  it('keeps the document presets at their old sizes, near enough to read the same', () => {
    expect(documentPx(100)).toBe(13.5)
    expect(documentPx(89)).toBeCloseTo(12, 0)
    expect(documentPx(115)).toBeCloseTo(15.5, 0)
    expect(documentPx(133)).toBeCloseTo(18, 0)
  })
})

describe('useSurfaceScale', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    document.documentElement.style.removeProperty('--interface-scale')
    mocks.AppearanceSettings.mockResolvedValue({ interfaceScale: 0, documentScale: 0, codeScale: 0, terminalScale: 0 })
    for (const write of [mocks.SetInterfaceScale, mocks.SetDocumentScale, mocks.SetCodeScale, mocks.SetTerminalScale]) {
      write.mockResolvedValue(undefined)
    }
  })

  it('hydrates every surface from one read and clamps what it finds', async () => {
    mocks.AppearanceSettings.mockResolvedValue({
      interfaceScale: 900,
      documentScale: 130,
      codeScale: 90,
      terminalScale: 115,
    })

    const { scales } = useSurfaceScale()
    await flushPromises()

    expect(mocks.AppearanceSettings).toHaveBeenCalledTimes(1)
    expect(scales.interface.value).toBe(150)
    expect(scales.document.value).toBe(130)
    expect(scales.code.value).toBe(90)
    expect(scales.terminal.value).toBe(115)
  })

  it('applies the interface and code scales to the root element', async () => {
    const { setScale } = useSurfaceScale()
    await flushPromises()

    setScale('interface', 130)
    setScale('code', 90)
    await flushPromises()

    const root = document.documentElement.style
    expect(root.getPropertyValue('--interface-scale')).toBe('1.3')
    expect(root.getPropertyValue('--code-scale')).toBe('0.9')
    expect(mocks.SetInterfaceScale).toHaveBeenCalledWith(130)
    expect(mocks.SetCodeScale).toHaveBeenCalledWith(90)
  })

  it('paints the cached interface scale before the file has been read', async () => {
    useSurfaceScale().setScale('interface', 120)
    await flushPromises()
    resetStores()

    mocks.AppearanceSettings.mockReturnValue(new Promise(() => {}))
    const { scales } = useSurfaceScale()
    await flushPromises()

    expect(scales.interface.value).toBe(120)
    expect(document.documentElement.style.getPropertyValue('--interface-scale')).toBe('1.2')
  })

  it('steps from the persisted scale when nothing has hydrated yet', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalScale: 130 })

    await useSurfaceScale().stepScale('terminal', 1)

    expect(useSurfaceScale().scales.terminal.value).toBe(150)
    expect(mocks.SetTerminalScale).toHaveBeenCalledTimes(1)
    expect(mocks.SetTerminalScale).toHaveBeenCalledWith(150)
  })

  it('resets a persisted scale even when nothing has hydrated yet', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ documentScale: 175 })

    await useSurfaceScale().resetScale('document')

    expect(mocks.SetDocumentScale).toHaveBeenCalledWith(100)
  })

  it('writes nothing at a limit', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ interfaceScale: 150 })

    await useSurfaceScale().stepScale('interface', 1)

    expect(useSurfaceScale().scales.interface.value).toBe(150)
    expect(mocks.SetInterfaceScale).not.toHaveBeenCalled()
  })

  it('moves each surface without touching the others', async () => {
    const { scales, stepScale } = useSurfaceScale()
    await flushPromises()

    await stepScale('document', 1)

    expect(scales.document.value).toBe(110)
    expect(scales.interface.value).toBe(100)
    expect(scales.code.value).toBe(100)
    expect(scales.terminal.value).toBe(100)
  })

  it('keeps the chosen scale when persisting fails', async () => {
    mocks.SetTerminalScale.mockRejectedValue(new Error('disk full'))
    const { scales, setScale } = useSurfaceScale()
    await flushPromises()

    setScale('terminal', 120)
    await flushPromises()

    expect(scales.terminal.value).toBe(120)
  })
})
