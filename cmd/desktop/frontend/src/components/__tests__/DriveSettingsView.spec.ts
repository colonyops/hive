import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DriveSettingsView from '../DriveSettingsView.vue'

const settingsBindings = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
  SetCanvasPageWidth: vi.fn(),
  ReviewLabels: vi.fn(),
  SetReviewLabels: vi.fn(),
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
  let stored = [{ title: 'Nit', icon: 'scissors' }]
  settingsBindings.ReviewLabels.mockImplementation(() => Promise.resolve(stored.map((label) => ({ ...label }))))
  settingsBindings.SetReviewLabels.mockImplementation((labels: typeof stored) => {
    stored = labels
    return Promise.resolve()
  })
})

describe('DriveSettingsView', () => {
  it('changes the document text size and line spacing', async () => {
    const wrapper = mount(DriveSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-drive-font-size-small"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="settings-drive-line-spacing-compact"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-testid="settings-drive-font-size-xl"]').trigger('click')
    await wrapper.get('[data-testid="settings-drive-line-spacing-relaxed"]').trigger('click')
    await flushPromises()

    expect(settingsBindings.SetCanvasFontSize).toHaveBeenCalledWith('xl')
    expect(settingsBindings.SetCanvasLineSpacing).toHaveBeenCalledWith('relaxed')
    const previewStyle = (wrapper.get('[data-testid="settings-canvas-preview-content"]').element as HTMLElement).style
    expect(previewStyle.getPropertyValue('--hv-font-size')).toBe('18px')
    expect(previewStyle.getPropertyValue('--hv-line-height')).toBe('1.85')
  })

  it('defaults the full-page width to narrow and saves a new choice', async () => {
    const wrapper = mount(DriveSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-drive-page-width-narrow"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-testid="settings-drive-page-width-full"]').trigger('click')
    await flushPromises()

    expect(settingsBindings.SetCanvasPageWidth).toHaveBeenCalledWith('full')
  })

  it('edits the comment labels', async () => {
    const wrapper = mount(DriveSettingsView, { attachTo: document.body })
    await flushPromises()

    const title = wrapper.get<HTMLInputElement>('[data-testid="settings-drive-label-1-title"]')
    expect(title.element.value).toBe('Nit')
    await title.setValue('Nitpick')
    await title.trigger('blur')
    await flushPromises()
    expect(settingsBindings.SetReviewLabels).toHaveBeenLastCalledWith([{ title: 'Nitpick', icon: 'scissors' }])

    await wrapper.get('[data-testid="settings-drive-labels-add"]').trigger('click')
    await flushPromises()
    const added = wrapper.get<HTMLInputElement>('[data-testid="settings-drive-label-2-title"]')
    await added.setValue('Nitpick')
    await added.trigger('blur')
    await flushPromises()
    expect(wrapper.get('[data-testid="settings-drive-labels-error"]').text()).toContain('listed twice')

    await added.setValue('Why')
    await added.trigger('blur')
    await flushPromises()
    expect(settingsBindings.SetReviewLabels).toHaveBeenLastCalledWith([
      { title: 'Nitpick', icon: 'scissors' },
      { title: 'Why', icon: 'tag' },
    ])

    await wrapper.get('[data-testid="settings-drive-labels-reset"]').trigger('click')
    await flushPromises()
    expect(settingsBindings.SetReviewLabels).toHaveBeenLastCalledWith([
      { title: 'Clarify this', icon: 'circle-help' },
      { title: 'Verify this', icon: 'search-check' },
      { title: 'Needs tests', icon: 'flask-conical' },
      { title: 'Looks good', icon: 'check' },
    ])
    wrapper.unmount()
  })

  it('drops a new label left without a title', async () => {
    const wrapper = mount(DriveSettingsView, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('[data-testid="settings-drive-labels-add"]').trigger('click')
    await wrapper.get('[data-testid="settings-drive-label-2-title"]').trigger('blur')
    await flushPromises()
    expect(wrapper.find('[data-testid="settings-drive-label-2"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
