import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CanvasSettingsView from '../CanvasSettingsView.vue'

const settingsBindings = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
  SetCanvasPageWidth: vi.fn(),
}))
vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settingsBindings,
)

beforeEach(() => {
  vi.clearAllMocks()
  settingsBindings.AppearanceSettings.mockResolvedValue({
    canvasFontSize: 'small',
    canvasLineSpacing: 'compact',
    canvasPageWidth: '',
  })
  settingsBindings.SetCanvasFontSize.mockResolvedValue(undefined)
  settingsBindings.SetCanvasLineSpacing.mockResolvedValue(undefined)
  settingsBindings.SetCanvasPageWidth.mockResolvedValue(undefined)
})

describe('CanvasSettingsView', () => {
  it('changes the global canvas text size and line spacing', async () => {
    const wrapper = mount(CanvasSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-canvas-font-size-small"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="settings-canvas-line-spacing-compact"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-testid="settings-canvas-font-size-xl"]').trigger('click')
    await wrapper.get('[data-testid="settings-canvas-line-spacing-relaxed"]').trigger('click')
    await flushPromises()

    expect(settingsBindings.SetCanvasFontSize).toHaveBeenCalledWith('xl')
    expect(settingsBindings.SetCanvasLineSpacing).toHaveBeenCalledWith('relaxed')
    const previewStyle = (wrapper.get('[data-testid="settings-canvas-preview-content"]').element as HTMLElement).style
    expect(previewStyle.getPropertyValue('--hv-font-size')).toBe('18px')
    expect(previewStyle.getPropertyValue('--hv-line-height')).toBe('1.85')
  })

  it('defaults the full-page width to narrow and saves a new choice', async () => {
    const wrapper = mount(CanvasSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-canvas-page-width-narrow"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-testid="settings-canvas-page-width-full"]').trigger('click')
    await flushPromises()

    expect(settingsBindings.SetCanvasPageWidth).toHaveBeenCalledWith('full')
  })
})
