import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useTerminalFont } from '../useTerminalFont'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTerminalFontWeights: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
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
    mocks.AppearanceSettings.mockResolvedValue({ terminalScale: 0 })
    mocks.SetTerminalFontWeights.mockResolvedValue(undefined)
  })

  afterEach(() => vi.restoreAllMocks())

  it('hydrates every setting from one read', async () => {
    mocks.AppearanceSettings.mockResolvedValue({
      terminalScale: 130,
      terminalFontFamily: 'Menlo',
      terminalFontWeight: 400,
      terminalFontWeightBold: 600,
      terminalLineHeight: 1.4,
      terminalLetterSpacing: 2,
    })

    const font = useTerminalFont()
    await settle()

    expect(font.px.value).toBe(17)
    expect(font.family.value).toBe('Menlo')
    expect(font.selectedFamily.value).toBe('Menlo')
    expect(font.weight.value).toBe(400)
    expect(font.weightBold.value).toBe(600)
    expect(font.lineHeight.value).toBe(1.4)
    expect(font.letterSpacing.value).toBe(2)
    expect(font.cellMetrics()).toBe('17|Menlo|400|600|1.4|2')
  })

  it('keeps the default size when the read fails', async () => {
    mocks.AppearanceSettings.mockRejectedValue(new Error('no file'))

    const font = useTerminalFont()
    await settle()

    expect(font.px.value).toBe(13)
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
