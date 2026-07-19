import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SettingsView from '../SettingsView.vue'
import { setTheme } from '../../composables/useTheme'

function mountSettings() {
  return mount(SettingsView)
}

beforeEach(() => {
  vi.stubGlobal('localStorage', (() => {
    const values = new Map<string, string>()
    return {
      get length() { return values.size },
      clear: () => values.clear(),
      getItem: (key: string) => values.get(key) ?? null,
      key: (index: number) => [...values.keys()][index] ?? null,
      removeItem: (key: string) => values.delete(key),
      setItem: (key: string, value: string) => values.set(key, value),
    }
  })())
  setTheme('dark') // deterministic starting theme for every test
})

afterEach(() => {
  delete document.documentElement.dataset.theme
  vi.unstubAllGlobals()
})

describe('SettingsView', () => {
  it('opens on the General category and lists all four categories in the rail', () => {
    const wrapper = mountSettings()

    expect(wrapper.find('[data-testid="settings-category-general"]').attributes('aria-current')).toBe('true')
    expect(wrapper.find('[data-testid="settings-category-appearance"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-category-integrations"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-category-advanced"]').exists()).toBe(true)

    expect(wrapper.find('[data-testid="settings-display-name"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-secret-input"]').exists()).toBe(false)
  })

  it('switching categories shows that category\'s section and hides the others', async () => {
    const wrapper = mountSettings()

    await wrapper.find('[data-testid="settings-category-integrations"]').trigger('click')

    expect(wrapper.find('[data-testid="settings-category-integrations"]').attributes('aria-current')).toBe('true')
    expect(wrapper.find('[data-testid="settings-secret-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-display-name"]').exists()).toBe(false) // General no longer shown

    await wrapper.find('[data-testid="settings-category-advanced"]').trigger('click')

    expect(wrapper.find('[data-testid="settings-log-level-info"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-secret-input"]').exists()).toBe(false) // Integrations no longer shown
  })

  it('the masked-secret reveal toggles the input between password and text', async () => {
    const wrapper = mountSettings()
    await wrapper.find('[data-testid="settings-category-integrations"]').trigger('click')

    const input = wrapper.find('[data-testid="settings-secret-input"]')
    expect(input.attributes('type')).toBe('password')

    await input.setValue('ghp_supersecret')
    await wrapper.find('[data-testid="settings-secret-input-reveal"]').trigger('click')

    expect(input.attributes('type')).toBe('text')
    expect((input.element as HTMLInputElement).value).toBe('ghp_supersecret')

    await wrapper.find('[data-testid="settings-secret-input-reveal"]').trigger('click')
    expect(input.attributes('type')).toBe('password')
  })

  it('the Appearance theme control reflects and drives the real useTheme composable', async () => {
    const wrapper = mountSettings()
    await wrapper.find('[data-testid="settings-category-appearance"]').trigger('click')

    // Starts reflecting the app's current theme (dark, set in beforeEach).
    expect(wrapper.find('[data-testid="settings-theme-toggle-dark"]').attributes('aria-selected')).toBe('true')

    await wrapper.find('[data-testid="settings-theme-toggle-gruvbox"]').trigger('click')

    // The control's own selected state updates...
    expect(wrapper.find('[data-testid="settings-theme-toggle-gruvbox"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="settings-theme-toggle-dark"]').attributes('aria-selected')).toBe('false')
    // ...and it actually changed the app's real theme, not just local state.
    expect(document.documentElement.dataset.theme).toBe('gruvbox')
    expect(localStorage.getItem('hive.theme')).toBe('gruvbox')
  })

  it('the font-size segmented control (local, unwired) tracks its own selection independent of theme', async () => {
    const wrapper = mountSettings()
    await wrapper.find('[data-testid="settings-category-appearance"]').trigger('click')

    expect(wrapper.find('[data-testid="settings-font-size-medium"]').attributes('aria-selected')).toBe('true')
    await wrapper.find('[data-testid="settings-font-size-large"]').trigger('click')
    expect(wrapper.find('[data-testid="settings-font-size-large"]').attributes('aria-selected')).toBe('true')
  })

  it('switch controls are focusable role="switch" buttons that flip aria-checked', async () => {
    const wrapper = mountSettings()

    const toggle = wrapper.find('[data-testid="settings-autostart"]')
    expect(toggle.attributes('role')).toBe('switch')
    expect(toggle.attributes('aria-checked')).toBe('false')

    await toggle.trigger('click')
    expect(toggle.attributes('aria-checked')).toBe('true')
  })

  it('the number stepper increments/decrements within min/max via its +/- buttons', async () => {
    const wrapper = mountSettings()
    await wrapper.find('[data-testid="settings-category-integrations"]').trigger('click')

    expect(wrapper.find('[data-testid="settings-poll-interval-value"]').text()).toContain('60')

    await wrapper.find('[data-testid="settings-poll-interval-increment"]').trigger('click')
    expect(wrapper.find('[data-testid="settings-poll-interval-value"]').text()).toContain('75')

    await wrapper.find('[data-testid="settings-poll-interval-decrement"]').trigger('click')
    await wrapper.find('[data-testid="settings-poll-interval-decrement"]').trigger('click')
    expect(wrapper.find('[data-testid="settings-poll-interval-value"]').text()).toContain('45')
  })

  it('a select/dropdown control updates its value', async () => {
    const wrapper = mountSettings()

    const select = wrapper.find('[data-testid="settings-default-view"]')
    await select.setValue('unread')
    expect((select.element as HTMLSelectElement).value).toBe('unread')
  })

  it('closes on the header Back-to-feed button and on Escape', async () => {
    const wrapper = mountSettings()

    await wrapper.find('[data-testid="settings-close"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(2)
  })
})
