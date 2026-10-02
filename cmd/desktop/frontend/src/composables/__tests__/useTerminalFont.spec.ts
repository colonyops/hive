import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTerminalFontSize: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetTerminalFontSize: mocks.SetTerminalFontSize,
}))

// The size is a module singleton, so each test resets modules and imports fresh
// rather than inheriting the previous one's ladder position.
beforeEach(() => {
  vi.resetModules()
  vi.clearAllMocks()
  mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 0 })
  mocks.SetTerminalFontSize.mockResolvedValue(undefined)
})

// Yielding to a macrotask drains the hydrate and persist chains without
// counting the microtask hops inside them.
async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

describe('useTerminalFont', () => {
  it('hydrates the pixel size the service resolved', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 17 })
    const { useTerminalFont } = await import('../useTerminalFont')

    const { px } = useTerminalFont()
    await settle()

    expect(px.value).toBe(17)
  })

  it('steps by two and holds at both ends', async () => {
    const { maxTerminalFontSizePx, minTerminalFontSizePx, stepTerminalFontSize, useTerminalFont } =
      await import('../useTerminalFont')
    const { px } = useTerminalFont()
    await settle()
    const start = px.value

    await stepTerminalFontSize(1)
    expect(px.value).toBe(start + 2)
    expect(mocks.SetTerminalFontSize).toHaveBeenLastCalledWith(start + 2)

    for (let i = 0; i < 100; i++) await stepTerminalFontSize(1)
    expect(px.value).toBe(maxTerminalFontSizePx)

    for (let i = 0; i < 100; i++) await stepTerminalFontSize(-1)
    expect(px.value).toBe(minTerminalFontSizePx)
  })

  it('clamps the final step onto the bound', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 63 })
    const { maxTerminalFontSizePx, stepTerminalFontSize, useTerminalFont } = await import('../useTerminalFont')
    const { px } = useTerminalFont()
    await settle()

    await stepTerminalFontSize(1)

    expect(px.value).toBe(maxTerminalFontSizePx)
  })

  it('resets to the default size', async () => {
    const { defaultTerminalFontSizePx, resetTerminalFontSize, setTerminalFontSize, useTerminalFont } =
      await import('../useTerminalFont')
    const { px } = useTerminalFont()
    await settle()

    setTerminalFontSize(32)
    await resetTerminalFontSize()

    expect(px.value).toBe(defaultTerminalFontSizePx)
  })

  it('resets a persisted size even when nothing has hydrated yet', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 18 })
    const { defaultTerminalFontSizePx, resetTerminalFontSize, useTerminalFont } = await import('../useTerminalFont')

    await resetTerminalFontSize()
    await settle()

    expect(useTerminalFont().px.value).toBe(defaultTerminalFontSizePx)
    expect(mocks.SetTerminalFontSize).toHaveBeenCalledWith(defaultTerminalFontSizePx)
  })

  it('steps from the persisted size when nothing has hydrated yet', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalFontSizePx: 16 })
    const { stepTerminalFontSize, useTerminalFont } = await import('../useTerminalFont')

    await stepTerminalFontSize(1)

    expect(useTerminalFont().px.value).toBe(18)
    expect(mocks.SetTerminalFontSize).toHaveBeenCalledTimes(1)
    expect(mocks.SetTerminalFontSize).toHaveBeenCalledWith(18)
  })

  it('keeps the chosen size when persisting fails', async () => {
    mocks.SetTerminalFontSize.mockRejectedValue(new Error('disk full'))
    const { setTerminalFontSize, useTerminalFont } = await import('../useTerminalFont')
    const { px } = useTerminalFont()
    await settle()

    setTerminalFontSize(14)
    await settle()

    expect(px.value).toBe(14)
  })

  it('does not let a slow settings read clobber a size chosen meanwhile', async () => {
    let resolveRead: (value: { terminalFontSizePx: number }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveRead = resolve
      }),
    )
    const { setTerminalFontSize, useTerminalFont } = await import('../useTerminalFont')
    const { px } = useTerminalFont()

    setTerminalFontSize(14)
    resolveRead({ terminalFontSizePx: 12 })
    await settle()

    expect(px.value).toBe(14)
  })
})
