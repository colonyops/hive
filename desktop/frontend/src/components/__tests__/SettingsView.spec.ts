import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import SettingsView from '../SettingsView.vue'
import { setTheme } from '../../composables/useTheme'

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/settingsservice', () => ({
  GithubSettings: vi.fn().mockResolvedValue({ pollIntervalSeconds: 60, minPollIntervalSeconds: 60 }),
  SetGithubSettings: vi.fn(),
}))

beforeEach(() => {
  localStorage.clear()
  setTheme('dark')
})

afterEach(() => {
  delete document.documentElement.dataset.theme
})

describe('SettingsView', () => {
  it('only exposes settings backed by application behavior', () => {
    const wrapper = mount(SettingsView, { props: { githubConnected: true, githubLogin: 'hayden', activeCategory: 'appearance' } })

    expect(wrapper.find('[data-testid="settings-category-appearance"]').attributes('aria-current')).toBe('true')
    expect(wrapper.find('[data-testid="settings-theme-toggle-dark"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-category-general"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-category-integrations"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-category-advanced"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-display-name"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-font-size"]').exists()).toBe(false)
  })

  it('reflects and changes the real application theme', async () => {
    const wrapper = mount(SettingsView, { props: { githubConnected: true, activeCategory: 'appearance' } })

    expect(wrapper.find('[data-testid="settings-theme-toggle-dark"]').attributes('aria-selected')).toBe('true')

    await wrapper.find('[data-testid="settings-theme-toggle-gruvbox"]').trigger('click')

    expect(wrapper.find('[data-testid="settings-theme-toggle-gruvbox"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="settings-theme-toggle-dark"]').attributes('aria-selected')).toBe('false')
    expect(document.documentElement.dataset.theme).toBe('gruvbox')
    await nextTick()
    expect(localStorage.getItem('hive.theme')).toBe('gruvbox')
  })

  it('shows the connected GitHub source and outlined future connections', async () => {
    const wrapper = mount(SettingsView, { props: { githubConnected: true, githubLogin: 'hayden', activeCategory: 'appearance' } })

    await wrapper.find('[data-testid="settings-category-integrations"]').trigger('click')
    expect(wrapper.emitted('select-category')).toEqual([['integrations']])
    await wrapper.setProps({ activeCategory: 'integrations' })

    expect(wrapper.find('[data-testid="integration-github-status"]').text()).toBe('Connected')
    expect(wrapper.find('[data-testid="integration-github"]').text()).toContain('Connected as hayden')
    for (const id of ['grafana', 'posthog', 'slack']) {
      expect(wrapper.find(`[data-testid="integration-${id}"] img`).exists()).toBe(true)
      expect(wrapper.find(`[data-testid="integration-${id}-add"]`).attributes('disabled')).toBeDefined()
      expect(wrapper.find(`[data-testid="integration-${id}"]`).text()).toContain('Coming soon')
    }
  })

  it('opens GitHub integration settings from the cog', async () => {
    const wrapper = mount(SettingsView, {
      props: { githubConnected: true, activeCategory: 'integrations' },
      global: { stubs: { Teleport: true } },
    })

    expect(wrapper.find('[data-testid="integration-github-configure"]').exists()).toBe(true)
    await wrapper.find('[data-testid="integration-github-configure"]').trigger('click')
    expect(wrapper.find('[data-testid="github-integration-drawer"]').exists()).toBe(true)
  })

  it('exposes a keybindings section that renders the editor', () => {
    const wrapper = mount(SettingsView, { props: { githubConnected: true, activeCategory: 'keybindings' } })

    expect(wrapper.find('[data-testid="settings-category-keybindings"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-keybindings"]').exists()).toBe(true)
  })

  it('closes from the header action and Escape', async () => {
    const wrapper = mount(SettingsView, { props: { githubConnected: true, activeCategory: 'appearance' } })

    await wrapper.find('[data-testid="settings-close"]').trigger('click')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(2)
  })
})
