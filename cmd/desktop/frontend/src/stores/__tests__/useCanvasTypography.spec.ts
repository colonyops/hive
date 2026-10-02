import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCanvasTypography } from '../useCanvasTypography'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => mocks)

describe('useCanvasTypography', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
    mocks.SetCanvasFontSize.mockResolvedValue(undefined)
    mocks.SetCanvasLineSpacing.mockResolvedValue(undefined)
  })

  it('hydrates both presets from one read and exposes their CSS values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'xl', canvasLineSpacing: 'relaxed' })

    const typography = useCanvasTypography()
    await flushPromises()

    expect(mocks.AppearanceSettings).toHaveBeenCalledTimes(1)
    expect(typography.fontSize.value).toBe('xl')
    expect(typography.fontSizePx.value).toBe(18)
    expect(typography.lineSpacing.value).toBe('relaxed')
    expect(typography.lineHeight.value).toBe(1.85)
  })

  it('persists selections while applying them immediately', async () => {
    const typography = useCanvasTypography()
    await flushPromises()

    typography.setFontSize('large')
    typography.setLineSpacing('compact')
    expect(typography.fontSizePx.value).toBe(15.5)
    expect(typography.lineHeight.value).toBe(1.45)
    await flushPromises()

    expect(mocks.SetCanvasFontSize).toHaveBeenCalledWith('large')
    expect(mocks.SetCanvasLineSpacing).toHaveBeenCalledWith('compact')
  })

  it('falls back from unknown stored values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'giant', canvasLineSpacing: 'wide' })

    const typography = useCanvasTypography()
    await flushPromises()

    expect(typography.fontSize.value).toBe('medium')
    expect(typography.lineSpacing.value).toBe('standard')
  })

  it('keeps a selection made while hydration is in flight', async () => {
    let resolveRead: (value: { canvasFontSize: string; canvasLineSpacing: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveRead = resolve
      }),
    )
    const typography = useCanvasTypography()

    typography.setFontSize('large')
    resolveRead({ canvasFontSize: 'small', canvasLineSpacing: 'compact' })
    await flushPromises()

    expect(typography.fontSize.value).toBe('large')
    expect(typography.lineSpacing.value).toBe('compact')
  })
})
