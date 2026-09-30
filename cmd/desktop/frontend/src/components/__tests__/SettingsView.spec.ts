import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import SettingsView from '../SettingsView.vue'
import { setTheme } from '../../composables/useTheme'
import { resetWebhookSettingsForTests } from '../../composables/useWebhookSettings'
import { applicationSettingsSections } from '../../router'

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  GithubSettings: vi.fn().mockResolvedValue({ pollIntervalSeconds: 60, minPollIntervalSeconds: 60 }),
  SetGithubSettings: vi.fn(),
  NotificationSettings: vi.fn().mockResolvedValue({ notificationsEnabled: true, systemNotificationsEnabled: true, notificationSound: true }),
  SetNotificationSettings: vi.fn(),
  AppearanceSettings: vi.fn().mockResolvedValue({ theme: '', terminalFontSizePx: 13, terminalFontFamily: '', terminalFontWeight: 0, terminalFontWeightBold: 0, terminalShowWindows: true, terminalPoolSize: 3 }),
  Fonts: vi.fn().mockResolvedValue({ all: [], monospace: [] }),
  SetTheme: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/notificationservice', () => ({
  PermissionStatus: vi.fn().mockResolvedValue('not-requested'),
  RequestNotificationPermission: vi.fn(),
}))
const listIntegrations = vi.hoisted(() => vi.fn())
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice', () => ({
  List: listIntegrations,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/githubservice', () => ({
  Status: vi.fn().mockResolvedValue({ state: 'connected', login: 'octocat', name: 'Octocat', avatarUrl: '', message: '' }),
  StartDeviceFlow: vi.fn(),
  CancelDeviceFlow: vi.fn(),
  SetToken: vi.fn(),
  Disconnect: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/grafanaservice', () => ({
  Connect: vi.fn(),
  Disconnect: vi.fn(),
}))
vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn().mockReturnValue(() => {}) },
  Browser: { OpenURL: vi.fn() },
}))

const webhookSettings = vi.hoisted(() => vi.fn())
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/webhookservice', () => ({
  Settings: webhookSettings,
  SetSettings: vi.fn(),
  GeneratePort: vi.fn(),
}))

beforeEach(() => {
  localStorage.clear()
  setTheme('dark')
  resetWebhookSettingsForTests()
  listIntegrations.mockResolvedValue([
    { key: 'github', title: 'GitHub source', stability: 'stable', provider: 'github', types: ['sources.github'], accounts: ['octocat'], envOverride: false },
    { key: 'sources.webhook', title: 'Webhook source', stability: 'stable', provider: '', types: ['sources.webhook'], accounts: [], envOverride: false },
  ])
  webhookSettings.mockResolvedValue({
    enabled: true,
    port: 24831,
    portMin: 20000,
    portMax: 32767,
    portOverridden: false,
    running: true,
    boundHost: '127.0.0.1',
    boundPort: 24831,
    baseUrl: 'http://127.0.0.1:24831/hooks/',
    startError: '',
    restartRequired: false,
  })
})

afterEach(() => {
  delete document.documentElement.dataset.theme
})

describe('SettingsView', () => {
  it('puts every routable section in exactly one nav group', () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'appearance' } })

    const rendered = wrapper.findAll('[data-testid^="settings-category-"]')
      .map((item) => item.attributes('data-testid')!.replace('settings-category-', ''))

    expect(rendered.slice(-3)).toEqual(['system', 'observability', 'about'])
    expect([...rendered].sort()).toEqual([...applicationSettingsSections].sort())
    expect(new Set(rendered).size).toBe(rendered.length)
  })

  it('only exposes settings backed by application behavior', () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'appearance' } })

    expect(wrapper.find('[data-testid="settings-category-appearance"]').attributes('aria-current')).toBe('true')
    expect(wrapper.find('[data-testid="settings-theme-dark"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-category-integrations"]').exists()).toBe(true)
    // Group headings are not categories: "Advanced" names a nav group, and a
    // pane of that name would be a junk drawer rather than a setting.
    expect(wrapper.find('[data-testid="settings-category-advanced"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-display-name"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-font-size"]').exists()).toBe(false)
  })

  // Appearance kept the terminal typography long after terminals outgrew it
  // (#222): a control belongs to the pane named for the surface it changes.
  it('leaves terminal typography to the Terminal pane', () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'appearance' } })

    expect(wrapper.find('[data-testid="settings-theme-dark"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-terminal-font-family"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-terminal-pool-size"]').exists()).toBe(false)
  })

  it('reflects and changes the real application theme', async () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'appearance' } })

    expect(wrapper.find('[data-testid="settings-theme-dark"]').attributes('aria-checked')).toBe('true')

    await wrapper.find('[data-testid="settings-theme-gruvbox"]').trigger('click')

    expect(wrapper.find('[data-testid="settings-theme-gruvbox"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.find('[data-testid="settings-theme-dark"]').attributes('aria-checked')).toBe('false')
    expect(document.documentElement.dataset.theme).toBe('gruvbox')
    await nextTick()
    expect(localStorage.getItem('hive.theme')).toBe('gruvbox')
  })

  it('shows the connected GitHub source', async () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'appearance' } })

    await wrapper.find('[data-testid="settings-category-integrations"]').trigger('click')
    expect(wrapper.emitted('select-category')).toEqual([['integrations']])
    await wrapper.setProps({ activeCategory: 'integrations' })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-github-status"]').text()).toBe('Connected')
    expect(wrapper.find('[data-testid="integration-github"]').text()).toContain('Connected as octocat')
  })

  // The card list is a projection of the Go connector registry. A hardcoded
  // "coming soon" list is what this replaced: it drifted from the registry in
  // both directions — promising connectors that did not exist, and silently
  // omitting ones that did.
  it('renders one card per registered connector and nothing else', async () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    expect(wrapper.findAll('[data-testid^="integration-"][data-testid$="-status"]')).toHaveLength(2)
    for (const id of ['grafana', 'posthog', 'slack']) {
      expect(wrapper.find(`[data-testid="integration-${id}"]`).exists()).toBe(false)
    }
  })

  // The card list's presentation/drawer maps (in SettingsView.vue) are keyed
  // by connector type and documented as incomplete by design: a type the
  // registry reports but the maps have not met yet still renders — generic
  // icon, no blurb, no configure gear — rather than being dropped from the
  // list. A connector added in Go before its presentation entry lands must
  // not silently disappear from Settings.
  //
  // The provider here is fictional on purpose. This used to name a real
  // connector that was not implemented yet, which broke the day it was — keep
  // it fictional so the test stays about the unknown-connector path.
  it('renders a card for a connector type its presentation maps do not know', async () => {
    listIntegrations.mockResolvedValue([
      { key: 'zzz-not-a-provider', title: 'Unmapped source', stability: 'experimental', provider: 'zzz-not-a-provider', types: ['sources.zzz'], accounts: [], envOverride: false },
    ])
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    const card = wrapper.find('[data-testid="integration-zzz-not-a-provider"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('Unmapped source')
    // No presentation entry means no blurb, not a crash or a missing card.
    expect(wrapper.find('[data-testid="integration-zzz-not-a-provider-status"]').text()).toBe('Not connected')
    // No drawer entry means no configure gear, rather than a dead button.
    expect(wrapper.find('[data-testid="integration-zzz-not-a-provider-configure"]').exists()).toBe(false)
  })

  it('reports a connector connected by an environment override', async () => {
    listIntegrations.mockResolvedValue([
      { key: 'github', title: 'GitHub source', stability: 'stable', provider: 'github', types: ['sources.github'], accounts: [], envOverride: true },
    ])
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-github-status"]').text()).toBe('Connected')
    expect(wrapper.find('[data-testid="integration-github"]').text()).toContain('HIVE_GITHUB_TOKEN')
  })

  it('reports a connector with no credential as not connected', async () => {
    listIntegrations.mockResolvedValue([
      { key: 'github', title: 'GitHub source', stability: 'stable', provider: 'github', types: ['sources.github'], accounts: [], envOverride: false },
    ])
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-github-status"]').text()).toBe('Not connected')
  })

  it('opens GitHub integration settings from the cog', async () => {
    const wrapper = mount(SettingsView, {
      props: { activeCategory: 'integrations' },
      global: { stubs: { Teleport: true } },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-github-configure"]').exists()).toBe(true)
    await wrapper.find('[data-testid="integration-github-configure"]').trigger('click')
    expect(wrapper.find('[data-testid="github-integration-drawer"]').exists()).toBe(true)
  })

  it('opens Grafana integration settings from the cog', async () => {
    listIntegrations.mockResolvedValue([
      { key: 'grafana', title: 'Grafana', stability: 'stable', provider: 'grafana', types: ['sources.grafana_alerts', 'sources.grafana_metrics'], accounts: [], envOverride: false },
    ])
    const wrapper = mount(SettingsView, {
      props: { activeCategory: 'integrations' },
      global: { stubs: { Teleport: true } },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-grafana-configure"]').exists()).toBe(true)
    await wrapper.find('[data-testid="integration-grafana-configure"]').trigger('click')
    expect(wrapper.find('[data-testid="grafana-integration-drawer"]').exists()).toBe(true)
  })

  it('shows the local webhook listener alongside the other integrations', async () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-webhook-status"]').text()).toBe('Running')
    expect(wrapper.find('[data-testid="integration-webhook"]').text()).toContain('http://127.0.0.1:24831/hooks/')
  })

  it('reports a disabled listener on the webhook card', async () => {
    webhookSettings.mockResolvedValue({
      enabled: false,
      port: 24831,
      portMin: 20000,
      portMax: 32767,
      portOverridden: false,
      running: false,
      boundHost: '127.0.0.1',
      boundPort: 0,
      baseUrl: 'http://127.0.0.1:24831/hooks/',
      startError: '',
      restartRequired: false,
    })
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-webhook-status"]').text()).toBe('Disabled')
  })

  it('a saved change the listener has not applied outranks what it is doing', async () => {
    // Disabled in settings but still bound: this session is running on
    // borrowed time, and the card must say so rather than "Running".
    webhookSettings.mockResolvedValue({
      enabled: false,
      port: 24831,
      portMin: 20000,
      portMax: 32767,
      portOverridden: false,
      running: true,
      boundHost: '127.0.0.1',
      boundPort: 24831,
      baseUrl: 'http://127.0.0.1:24831/hooks/',
      startError: '',
      restartRequired: true,
    })
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-webhook-status"]').text()).toBe('Restart needed')
  })

  it('reports a bind failure on the webhook card', async () => {
    webhookSettings.mockResolvedValue({
      enabled: true,
      port: 24831,
      portMin: 20000,
      portMax: 32767,
      portOverridden: false,
      running: false,
      boundHost: '127.0.0.1',
      boundPort: 0,
      baseUrl: 'http://127.0.0.1:24831/hooks/',
      startError: 'webhook listener: address already in use',
      restartRequired: true,
    })
    const wrapper = mount(SettingsView, { props: { activeCategory: 'integrations' } })
    await flushPromises()

    expect(wrapper.find('[data-testid="integration-webhook-status"]').text()).toBe('Port in use')
  })

  it('opens webhook settings from the cog', async () => {
    const wrapper = mount(SettingsView, {
      props: { activeCategory: 'integrations' },
      global: { stubs: { Teleport: true } },
    })
    await flushPromises()

    await wrapper.find('[data-testid="integration-webhook-configure"]').trigger('click')
    expect(wrapper.find('[data-testid="webhook-integration-drawer"]').exists()).toBe(true)
  })

  it('exposes a notifications category that renders the notification settings', () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'notifications' } })

    expect(wrapper.find('[data-testid="settings-category-notifications"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="notification-settings"]').exists()).toBe(true)
  })

  it('exposes a keybindings section that renders the editor', () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'keybindings' } })

    expect(wrapper.find('[data-testid="settings-category-keybindings"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-keybindings"]').exists()).toBe(true)
  })

  it('closes on Escape', async () => {
    const wrapper = mount(SettingsView, { props: { activeCategory: 'appearance' } })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
