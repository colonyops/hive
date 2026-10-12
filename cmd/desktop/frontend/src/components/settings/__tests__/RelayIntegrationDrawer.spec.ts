import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RelayIntegrationDrawer from '../RelayIntegrationDrawer.vue'

const mocks = vi.hoisted(() => ({
  ConnectWithKey: vi.fn(),
  Disconnect: vi.fn(),
  List: vi.fn(),
  On: vi.fn(),
  OpenURL: vi.fn(),
}))

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/relayservice', () => ({
  ConnectWithKey: mocks.ConnectWithKey,
  Disconnect: mocks.Disconnect,
}))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice',
  () => ({
    List: mocks.List,
  }),
)
vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Browser: { OpenURL: mocks.OpenURL },
}))

const ACCOUNT = 'relay.hivedesktop.com-you@example.com'

function connectedAccounts(...accounts: string[]) {
  mocks.List.mockResolvedValue([
    {
      key: 'relay',
      title: 'Hive Relay',
      stability: 'experimental',
      provider: 'relay',
      types: ['sources.relay'],
      accounts,
      envOverride: false,
    },
  ])
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  connectedAccounts()
})

function mountDrawer() {
  return mount(RelayIntegrationDrawer, { global: { stubs: { Teleport: true } } })
}

describe('RelayIntegrationDrawer', () => {
  it('seeds the hosted relay URL', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.find('[data-testid="relay-connected-empty"]').exists()).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-testid="relay-connect-url"]').element.value).toBe(
      'https://relay.hivedesktop.com',
    )
  })

  it('connects with an API key, then clears the inputs', async () => {
    mocks.ConnectWithKey.mockResolvedValue({
      account: ACCOUNT,
      url: 'https://relay.hivedesktop.com',
      email: 'you@example.com',
    })
    const wrapper = mountDrawer()
    await flushPromises()

    await wrapper.find('[data-testid="relay-connect-token"]').setValue('rk_key')
    await wrapper.find('[data-testid="relay-connect-submit"]').trigger('click')
    await flushPromises()

    expect(mocks.ConnectWithKey).toHaveBeenCalledWith('https://relay.hivedesktop.com', 'rk_key')
    expect(wrapper.get<HTMLInputElement>('[data-testid="relay-connect-token"]').element.value).toBe('')
  })

  it('surfaces a rejected key', async () => {
    mocks.ConnectWithKey.mockRejectedValue(new Error('relay rejected the key'))
    const wrapper = mountDrawer()
    await flushPromises()

    await wrapper.find('[data-testid="relay-connect-token"]').setValue('bad')
    await wrapper.find('[data-testid="relay-connect-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="relay-connect-error"]').text()).toContain('rejected')
  })

  it('lists a connected account and disconnects it', async () => {
    connectedAccounts(ACCOUNT)
    mocks.Disconnect.mockResolvedValue(undefined)
    const wrapper = mountDrawer()
    await flushPromises()

    expect(wrapper.find(`[data-testid="relay-connected-${ACCOUNT}"]`).exists()).toBe(true)
    await wrapper.find(`[data-testid="relay-disconnect-${ACCOUNT}"]`).trigger('click')
    await flushPromises()

    expect(mocks.Disconnect).toHaveBeenCalledWith(ACCOUNT)
  })

  // The relay has no web page that mints keys, so there is nothing to deep-link.
  it('links to the docs and offers no token page', async () => {
    const wrapper = mountDrawer()
    await flushPromises()

    expect(wrapper.find('[data-testid="relay-connect-help"]').text()).toContain('hive-relay tokens create')
    expect(wrapper.find('[data-testid="relay-connect-instance-link"]').exists()).toBe(false)
    await wrapper.find('[data-testid="relay-connect-docs"]').trigger('click')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://hivedesktop.com/desktop/inbox/sources/#hive-relay')
  })
})
