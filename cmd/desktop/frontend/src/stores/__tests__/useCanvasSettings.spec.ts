import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCanvasSettings } from '../useCanvasSettings'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
  SetCanvasPageWidth: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => mocks)

describe('useCanvasSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ canvasLineSpacing: '' })
    mocks.SetCanvasLineSpacing.mockResolvedValue(undefined)
    mocks.SetCanvasPageWidth.mockResolvedValue(undefined)
  })

  it('hydrates the reading settings and exposes their CSS values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ documentScale: 200, canvasLineSpacing: 'relaxed' })

    const typography = useCanvasSettings()
    await flushPromises()

    expect(typography.fontSizePx.value).toBe(27)
    expect(typography.lineSpacing.value).toBe('relaxed')
    expect(typography.lineHeight.value).toBe(1.85)
  })

  it('persists selections while applying them immediately', async () => {
    const typography = useCanvasSettings()
    await flushPromises()

    typography.setLineSpacing('compact')
    expect(typography.lineHeight.value).toBe(1.45)
    await flushPromises()

    expect(mocks.SetCanvasLineSpacing).toHaveBeenCalledWith('compact')
  })

  it('maps the page width preset to a measure and persists a change', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ canvasLineSpacing: '', canvasPageWidth: 'wide' })

    const settings = useCanvasSettings()
    await flushPromises()
    expect(settings.pageWidthClass.value).toBe('max-w-[1040px]')

    settings.setPageWidth('full')
    expect(settings.pageWidthClass.value).toBe('max-w-none')
    await flushPromises()
    expect(mocks.SetCanvasPageWidth).toHaveBeenCalledWith('full')
  })

  it('falls back from unknown stored values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({
      canvasLineSpacing: 'wide',
      canvasPageWidth: 'huge',
    })

    const typography = useCanvasSettings()
    await flushPromises()

    expect(typography.lineSpacing.value).toBe('standard')
    expect(typography.pageWidth.value).toBe('narrow')
  })

  it('keeps a selection made while hydration is in flight', async () => {
    let resolveRead: (value: { canvasLineSpacing: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveRead = resolve
      }),
    )
    const typography = useCanvasSettings()

    typography.setLineSpacing('relaxed')
    resolveRead({ canvasLineSpacing: 'compact' })
    await flushPromises()

    expect(typography.lineSpacing.value).toBe('relaxed')
  })
})
