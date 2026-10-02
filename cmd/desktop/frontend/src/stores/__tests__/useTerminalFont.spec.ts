import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import {
  defaultTerminalFontSizePx,
  maxTerminalFontSizePx,
  minTerminalFontSizePx,
  useTerminalFont,
} from '../useTerminalFont'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTerminalFontSize: vi.fn(),
  SetTerminalFontWeights: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetTerminalFontSize: mocks.SetTerminalFontSize,
  SetTerminalFontWeights: mocks.SetTerminalFontWeights,
}))

// Yielding to a macrotask drains the hydrate and persist chains without
// counting the microtask hops inside them.
async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

describe('useTerminalFont', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 0 })
    mocks.SetTerminalFontSize.mockResolvedValue(undefined)
    mocks.SetTerminalFontWeights.mockResolvedValue(undefined)
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => vi.restoreAllMocks())

  it('hydrates every setting from one read', async () => {
    mocks.AppearanceSettings.mockResolvedValue({
      terminalFontSizePx: 17,
      terminalFontFamily: 'Menlo',
      terminalFontWeight: 400,
      terminalFontWeightBold: 600,
      terminalLineHeight: 1.4,
      terminalLetterSpacing: 2,
    })

    const font = useTerminalFont()
    await settle()

    expect(mocks.AppearanceSettings).toHaveBeenCalledTimes(1)
    expect(font.px.value).toBe(17)
    expect(font.family.value).toBe('Menlo')
    expect(font.selectedFamily.value).toBe('Menlo')
    expect(font.weight.value).toBe(400)
    expect(font.weightBold.value).toBe(600)
    expect(font.lineHeight.value).toBe(1.4)
    expect(font.letterSpacing.value).toBe(2)
    expect(font.cellMetrics()).toBe('17|Menlo|400|600|1.4|2')
  })

  it('keeps the defaults and warns once when the read fails', async () => {
    mocks.AppearanceSettings.mockRejectedValue(new Error('no file'))

    const font = useTerminalFont()
    await settle()

    expect(font.px.value).toBe(defaultTerminalFontSizePx)
    expect(console.warn).toHaveBeenCalledTimes(1)
  })

  it('steps by two and holds at both ends', async () => {
    const font = useTerminalFont()
    await settle()
    const start = font.px.value

    await font.stepFontSize(1)
    expect(font.px.value).toBe(start + 2)
    expect(mocks.SetTerminalFontSize).toHaveBeenLastCalledWith(start + 2)

    for (let i = 0; i < 100; i++) await font.stepFontSize(1)
    expect(font.px.value).toBe(maxTerminalFontSizePx)

    for (let i = 0; i < 100; i++) await font.stepFontSize(-1)
    expect(font.px.value).toBe(minTerminalFontSizePx)
  })

  it('clamps the final step onto the bound', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 63 })
    const font = useTerminalFont()
    await settle()

    await font.stepFontSize(1)

    expect(font.px.value).toBe(maxTerminalFontSizePx)
  })

  it('resets to the default size', async () => {
    const font = useTerminalFont()
    await settle()

    font.setFontSize(32)
    await font.resetFontSize()

    expect(font.px.value).toBe(defaultTerminalFontSizePx)
  })

  it('resets a persisted size even when nothing has hydrated yet', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 18 })

    await useTerminalFont().resetFontSize()
    await settle()

    expect(useTerminalFont().px.value).toBe(defaultTerminalFontSizePx)
    expect(mocks.SetTerminalFontSize).toHaveBeenCalledWith(defaultTerminalFontSizePx)
  })

  it('steps from the persisted size when nothing has hydrated yet', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 16 })

    await useTerminalFont().stepFontSize(1)

    expect(useTerminalFont().px.value).toBe(18)
    expect(mocks.SetTerminalFontSize).toHaveBeenCalledTimes(1)
    expect(mocks.SetTerminalFontSize).toHaveBeenCalledWith(18)
  })

  it('keeps the chosen size when persisting fails', async () => {
    mocks.SetTerminalFontSize.mockRejectedValue(new Error('disk full'))
    const font = useTerminalFont()
    await settle()

    font.setFontSize(14)
    await settle()

    expect(font.px.value).toBe(14)
  })

  it('does not let a slow settings read clobber a size chosen meanwhile', async () => {
    let resolveRead: (value: { terminalFontSizePx: number }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveRead = resolve
      }),
    )
    const font = useTerminalFont()

    font.setFontSize(14)
    resolveRead({ terminalFontSizePx: 12 })
    await settle()

    expect(font.px.value).toBe(14)
  })

  it('persists both weights whichever one changes', async () => {
    const font = useTerminalFont()
    await settle()

    font.setFontWeight(400)
    await settle()
    expect(mocks.SetTerminalFontWeights).toHaveBeenLastCalledWith(400, 700)

    font.setFontWeightBold(600)
    await settle()
    expect(mocks.SetTerminalFontWeights).toHaveBeenLastCalledWith(400, 600)
  })
})
