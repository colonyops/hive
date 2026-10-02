import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import WebhookIntegrationDrawer from '../WebhookIntegrationDrawer.vue'

const getSettings = vi.hoisted(() => vi.fn())
const setSettings = vi.hoisted(() => vi.fn())
const generatePort = vi.hoisted(() => vi.fn())
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/webhookservice', () => ({
  Settings: getSettings,
  SetSettings: setSettings,
  GeneratePort: generatePort,
}))

function settings(overrides: Record<string, unknown> = {}) {
  return {
    enabled: true,
    host: '127.0.0.1',
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
    ...overrides,
  }
}

async function open(overrides: Record<string, unknown> = {}) {
  getSettings.mockResolvedValue(settings(overrides))
  const wrapper = mount(WebhookIntegrationDrawer, { global: { stubs: { Teleport: true } } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  setSettings.mockResolvedValue(undefined)
})

describe('WebhookIntegrationDrawer', () => {
  it('shows the running listener and its base URL', async () => {
    const wrapper = await open()

    expect(wrapper.get('[data-testid="webhook-settings-status"]').text()).toContain('Running')
    expect(wrapper.get('[data-testid="webhook-settings-status"]').text()).toContain('127.0.0.1:24831')
    expect(wrapper.get('[data-testid="webhook-settings-base-url-value"]').text()).toBe('http://127.0.0.1:24831/hooks/')
    expect((wrapper.get('[data-testid="webhook-settings-port-input"]').element as HTMLInputElement).value).toBe('24831')
    expect(wrapper.find('[data-testid="webhook-settings-restart-note"]').exists()).toBe(false)
  })

  it('surfaces a bind failure rather than leaving it in the log', async () => {
    const wrapper = await open({ running: false, boundPort: 0, startError: 'webhook listener: address already in use' })

    expect(wrapper.get('[data-testid="webhook-settings-status"]').text()).toContain('Failed to start')
    expect(wrapper.get('[data-testid="webhook-settings-status"]').text()).toContain('address already in use')
  })

  it('persists the port and enabled state, then flags the pending restart', async () => {
    const wrapper = await open()

    await wrapper.get('[data-testid="webhook-settings-port-input"]').setValue('27100')
    // A pending change is a restart the user has not taken yet.
    expect(wrapper.find('[data-testid="webhook-settings-restart-note"]').exists()).toBe(true)

    getSettings.mockResolvedValue(settings({ port: 27100, restartRequired: true }))
    await wrapper.get('[data-testid="webhook-settings-save"]').trigger('click')
    await flushPromises()

    expect(setSettings).toHaveBeenCalledWith(expect.objectContaining({ enabled: true, port: 27100 }))
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('disabling the listener is saved as an explicit false', async () => {
    const wrapper = await open()

    await wrapper.get('[data-testid="webhook-settings-enabled"]').trigger('click')
    getSettings.mockResolvedValue(settings({ enabled: false, restartRequired: true }))
    await wrapper.get('[data-testid="webhook-settings-save"]').trigger('click')
    await flushPromises()

    expect(setSettings).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }))
  })

  it('rejects a port outside the bindable range without calling the backend', async () => {
    const wrapper = await open()

    await wrapper.get('[data-testid="webhook-settings-port-input"]').setValue('80')

    expect(wrapper.find('[data-testid="webhook-settings-port-error"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="webhook-settings-save"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="webhook-settings-save"]').trigger('click')
    expect(setSettings).not.toHaveBeenCalled()
  })

  it('generates a new random port into the field without persisting it', async () => {
    const wrapper = await open()
    generatePort.mockResolvedValue(30512)

    await wrapper.get('[data-testid="webhook-settings-port-generate"]').trigger('click')
    await flushPromises()

    expect((wrapper.get('[data-testid="webhook-settings-port-input"]').element as HTMLInputElement).value).toBe('30512')
    expect(setSettings).not.toHaveBeenCalled()
  })

  it('locks the port when the environment override is in force', async () => {
    const wrapper = await open({ portOverridden: true })

    expect(wrapper.get('[data-testid="webhook-settings-port-input"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="webhook-settings-port-generate"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="webhook-settings-port-hint"]').text()).toContain('HIVE_DESKTOP_HTTP_PORT')
  })

  it('keeps the drawer open and reports a rejected save', async () => {
    const wrapper = await open()
    setSettings.mockRejectedValue(new Error('port must be between 1024 and 65535'))

    await wrapper.get('[data-testid="webhook-settings-save"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="webhook-settings-error"]').text()).toContain(
      'port must be between 1024 and 65535',
    )
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})
