import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ServerSettingsView from '../ServerSettingsView.vue'

const mocks = vi.hoisted(() => ({ Servers: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  Servers: mocks.Servers,
}))

async function mountView() {
  const wrapper = mount(ServerSettingsView)
  await flushPromises()
  return wrapper
}

describe('ServerSettingsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows how each server answered its check', async () => {
    mocks.Servers.mockResolvedValue([
      { name: 'box', ssh: 'ada@box', state: 'connected', version: '1.2.3', roundTripMs: 42 },
      { name: 'down', ssh: 'ada@down', state: 'offline', error: 'ssh: connection refused', roundTripMs: 0 },
    ])
    const wrapper = await mountView()

    expect(wrapper.get('[data-testid="settings-server-box-state"]').text()).toBe('Connected')
    expect(wrapper.get('[data-testid="settings-server-box"]').text()).toContain('ssh ada@box · hive 1.2.3 · 42 ms')
    expect(wrapper.get('[data-testid="settings-server-down-state"]').text()).toBe('Offline')
    expect(wrapper.get('[data-testid="settings-server-down"]').text()).toContain('ssh: connection refused')
  })

  it('says when no servers are configured', async () => {
    mocks.Servers.mockResolvedValue([])
    const wrapper = await mountView()
    expect(wrapper.find('[data-testid="settings-servers-empty"]').exists()).toBe(true)
  })

  it('shows a check that fails, and checks again on request', async () => {
    mocks.Servers.mockRejectedValue(new Error('no fleet'))
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('no fleet')
    expect(wrapper.find('[data-testid="settings-servers-empty"]').exists()).toBe(false)

    mocks.Servers.mockResolvedValue([{ name: 'box', ssh: 'ada@box', state: 'connected', roundTripMs: 1 }])
    await wrapper.get('[data-testid="settings-servers-check"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('no fleet')
    expect(wrapper.find('[data-testid="settings-server-box"]').exists()).toBe(true)
  })
})
