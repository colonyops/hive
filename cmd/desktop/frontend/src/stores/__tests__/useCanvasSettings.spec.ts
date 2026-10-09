import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../defineStore'
import { useCanvasSettings } from '../useCanvasSettings'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
  SetCanvasPageWidth: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => mocks)

describe('useCanvasSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
    mocks.SetCanvasFontSize.mockResolvedValue(undefined)
    mocks.SetCanvasLineSpacing.mockResolvedValue(undefined)
    mocks.SetCanvasPageWidth.mockResolvedValue(undefined)
  })

  it('hydrates both presets from one read and exposes their CSS values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'xl', canvasLineSpacing: 'relaxed' })

    const typography = useCanvasSettings()
    await flushPromises()

    expect(mocks.AppearanceSettings).toHaveBeenCalledTimes(1)
    expect(typography.fontSize.value).toBe('xl')
    expect(typography.fontSizePx.value).toBe(18)
    expect(typography.lineSpacing.value).toBe('relaxed')
    expect(typography.lineHeight.value).toBe(1.85)
  })

  it('persists selections while applying them immediately', async () => {
    const typography = useCanvasSettings()
    await flushPromises()

    typography.setFontSize('large')
    typography.setLineSpacing('compact')
    expect(typography.fontSizePx.value).toBe(15.5)
    expect(typography.lineHeight.value).toBe(1.45)
    await flushPromises()

    expect(mocks.SetCanvasFontSize).toHaveBeenCalledWith('large')
    expect(mocks.SetCanvasLineSpacing).toHaveBeenCalledWith('compact')
  })

  it('maps the page width preset to a measure and persists a change', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '', canvasPageWidth: 'wide' })

    const settings = useCanvasSettings()
    await flushPromises()
    expect(settings.pageWidthClass.value).toBe('max-w-[1040px]')

    settings.setPageWidth('full')
    expect(settings.pageWidthClass.value).toBe('max-w-none')
    await flushPromises()
    expect(mocks.SetCanvasPageWidth).toHaveBeenCalledWith('full')
  })

  it('restores the global front matter disclosure state from localStorage', async () => {
    const settings = useCanvasSettings()
    expect(settings.frontmatterExpanded.value).toBe(true)

    settings.toggleFrontmatter()
    await flushPromises()
    expect(localStorage.getItem('hive.canvas.frontmatter.expanded')).toBe('false')

    resetStores()
    expect(useCanvasSettings().frontmatterExpanded.value).toBe(false)
  })

  it('falls back from unknown stored values', async () => {
    mocks.AppearanceSettings.mockResolvedValue({
      canvasFontSize: 'giant',
      canvasLineSpacing: 'wide',
      canvasPageWidth: 'huge',
    })

    const typography = useCanvasSettings()
    await flushPromises()

    expect(typography.fontSize.value).toBe('medium')
    expect(typography.lineSpacing.value).toBe('standard')
    expect(typography.pageWidth.value).toBe('narrow')
  })

  it('keeps a selection made while hydration is in flight', async () => {
    let resolveRead: (value: { canvasFontSize: string; canvasLineSpacing: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveRead = resolve
      }),
    )
    const typography = useCanvasSettings()

    typography.setFontSize('large')
    resolveRead({ canvasFontSize: 'small', canvasLineSpacing: 'compact' })
    await flushPromises()

    expect(typography.fontSize.value).toBe('large')
    expect(typography.lineSpacing.value).toBe('compact')
  })
})
