import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => mocks)

beforeEach(() => {
  vi.resetModules()
  vi.clearAllMocks()
  mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
  mocks.SetCanvasFontSize.mockResolvedValue(undefined)
  mocks.SetCanvasLineSpacing.mockResolvedValue(undefined)
})

async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

describe('useCanvasTypography', () => {
  it('hydrates the stored presets and exposes their CSS values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'xl', canvasLineSpacing: 'relaxed' })
    const { useCanvasTypography } = await import('../useCanvasTypography')

    const typography = useCanvasTypography()
    await settle()

    expect(typography.fontSize.value).toBe('xl')
    expect(typography.fontSizePx.value).toBe(18)
    expect(typography.lineSpacing.value).toBe('relaxed')
    expect(typography.lineHeight.value).toBe(1.85)
  })

  it('persists selections while applying them immediately', async () => {
    const { setCanvasFontSize, setCanvasLineSpacing, useCanvasTypography } = await import('../useCanvasTypography')
    const typography = useCanvasTypography()
    await settle()

    setCanvasFontSize('large')
    setCanvasLineSpacing('compact')
    await settle()

    expect(typography.fontSizePx.value).toBe(15.5)
    expect(typography.lineHeight.value).toBe(1.45)
    expect(mocks.SetCanvasFontSize).toHaveBeenCalledWith('large')
    expect(mocks.SetCanvasLineSpacing).toHaveBeenCalledWith('compact')
  })

  it('falls back from unknown stored values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'giant', canvasLineSpacing: 'wide' })
    const { useCanvasTypography } = await import('../useCanvasTypography')

    const typography = useCanvasTypography()
    await settle()

    expect(typography.fontSize.value).toBe('medium')
    expect(typography.lineSpacing.value).toBe('standard')
  })

  it('keeps a selection made while hydration is in flight', async () => {
    let resolveRead: (value: { canvasFontSize: string; canvasLineSpacing: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(new Promise((resolve) => { resolveRead = resolve }))
    const { setCanvasFontSize, useCanvasTypography } = await import('../useCanvasTypography')
    const typography = useCanvasTypography()

    setCanvasFontSize('large')
    resolveRead({ canvasFontSize: 'small', canvasLineSpacing: 'compact' })
    await settle()

    expect(typography.fontSize.value).toBe('large')
    expect(typography.lineSpacing.value).toBe('compact')
  })
})
