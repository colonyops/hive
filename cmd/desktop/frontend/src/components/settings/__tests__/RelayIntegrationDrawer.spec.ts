import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import RelayIntegrationDrawer from '../RelayIntegrationDrawer.vue'

const mocks = vi.hoisted(() => ({
  ConnectWithKey: vi.fn(),
  Disconnect: vi.fn(),
  Login: vi.fn(),
  Methods: vi.fn(),
  StartEmail: vi.fn(),
  VerifyEmail: vi.fn(),
  List: vi.fn(),
  On: vi.fn(),
  OpenURL: vi.fn(),
}))

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/relayservice', () => ({
  ConnectWithKey: mocks.ConnectWithKey,
  Disconnect: mocks.Disconnect,
  Login: mocks.Login,
  Methods: mocks.Methods,
  StartEmail: mocks.StartEmail,
  VerifyEmail: mocks.VerifyEmail,
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

const URL = 'https://relay.hivedesktop.com'
const ACCOUNT = 'relay.hivedesktop.com-you@example.com'
const INSTANCE = { account: ACCOUNT, url: URL, email: 'you@example.com' }
const ALL = { email_code: true, password: true, api_key: true }

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
  mocks.Methods.mockResolvedValue(ALL)
  connectedAccounts()
})

async function mountDrawer() {
  const wrapper = mount(RelayIntegrationDrawer, { global: { stubs: { Teleport: true } } })
  await flushPromises()
  return wrapper
}

function has(wrapper: VueWrapper, testid: string): boolean {
  return wrapper.find(`[data-testid="${testid}"]`).exists()
}

async function click(wrapper: VueWrapper, testid: string): Promise<void> {
  await wrapper.get(`[data-testid="${testid}"]`).trigger('click')
  await flushPromises()
}

describe('RelayIntegrationDrawer', () => {
  it('reads the methods of the seeded hosted relay', async () => {
    const wrapper = await mountDrawer()
    expect(has(wrapper, 'relay-connected-empty')).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-testid="relay-connect-url"]').element.value).toBe(URL)
    expect(mocks.Methods).toHaveBeenCalledWith(URL)
  })

  it('offers every method the relay reports', async () => {
    const wrapper = await mountDrawer()
    expect(has(wrapper, 'relay-method-email')).toBe(true)
    expect(has(wrapper, 'relay-method-password')).toBe(true)
    expect(has(wrapper, 'relay-method-key')).toBe(true)
    expect(has(wrapper, 'relay-connect-email')).toBe(true)
    expect(has(wrapper, 'relay-send-code')).toBe(true)
  })

  it('shows the API key form directly when it is the only method', async () => {
    mocks.Methods.mockResolvedValue({ email_code: false, password: false, api_key: true })
    const wrapper = await mountDrawer()
    expect(has(wrapper, 'relay-method-key')).toBe(false)
    expect(has(wrapper, 'relay-method-email')).toBe(false)
    expect(has(wrapper, 'relay-connect-token')).toBe(true)
    expect(has(wrapper, 'relay-connect-email')).toBe(false)
  })

  it('shows the email form directly when it is the only method', async () => {
    mocks.Methods.mockResolvedValue({ email_code: true, password: false, api_key: false })
    const wrapper = await mountDrawer()
    expect(has(wrapper, 'relay-method-email')).toBe(false)
    expect(has(wrapper, 'relay-send-code')).toBe(true)
    expect(has(wrapper, 'relay-connect-token')).toBe(false)
    expect(has(wrapper, 'relay-connect-password')).toBe(false)
  })

  it('re-reads the methods for an edited URL', async () => {
    const wrapper = await mountDrawer()
    const input = wrapper.get('[data-testid="relay-connect-url"]')
    await input.setValue('https://relay.example.com')
    expect(has(wrapper, 'relay-send-code')).toBe(false)

    await input.trigger('blur')
    await flushPromises()

    expect(mocks.Methods).toHaveBeenLastCalledWith('https://relay.example.com')
    expect(has(wrapper, 'relay-send-code')).toBe(true)
  })

  it('signs in with an emailed code', async () => {
    mocks.StartEmail.mockResolvedValue(undefined)
    mocks.VerifyEmail.mockResolvedValue(INSTANCE)
    const wrapper = await mountDrawer()

    await wrapper.get('[data-testid="relay-connect-email"]').setValue('you@example.com')
    await click(wrapper, 'relay-send-code')
    expect(mocks.StartEmail).toHaveBeenCalledWith(URL, 'you@example.com')

    await wrapper.get('[data-testid="relay-connect-code"]').setValue('12345678')
    await click(wrapper, 'relay-verify')
    expect(mocks.VerifyEmail).toHaveBeenCalledWith(URL, 'you@example.com', '12345678')

    expect(has(wrapper, 'relay-connect-code')).toBe(false)
    expect(wrapper.get<HTMLInputElement>('[data-testid="relay-connect-email"]').element.value).toBe('')
  })

  it('shows a wrong code and sends a new one', async () => {
    mocks.StartEmail.mockResolvedValue(undefined)
    mocks.VerifyEmail.mockRejectedValue(new Error('invalid code; 2 attempts left'))
    const wrapper = await mountDrawer()

    await wrapper.get('[data-testid="relay-connect-email"]').setValue('you@example.com')
    await click(wrapper, 'relay-send-code')
    await wrapper.get('[data-testid="relay-connect-code"]').setValue('00000000')
    await click(wrapper, 'relay-verify')

    expect(wrapper.get('[data-testid="relay-error"]').text()).toContain('2 attempts left')
    expect(has(wrapper, 'relay-connect-code')).toBe(true)

    await click(wrapper, 'relay-resend')
    expect(mocks.StartEmail).toHaveBeenCalledTimes(2)
  })

  it('surfaces a refused email', async () => {
    mocks.StartEmail.mockRejectedValue(new Error('email is not allowed on this relay'))
    const wrapper = await mountDrawer()

    await wrapper.get('[data-testid="relay-connect-email"]').setValue('no@example.com')
    await click(wrapper, 'relay-send-code')

    expect(wrapper.get('[data-testid="relay-error"]').text()).toContain('not allowed')
    expect(has(wrapper, 'relay-connect-code')).toBe(false)
  })

  it('signs in with a password', async () => {
    mocks.Login.mockResolvedValue(INSTANCE)
    const wrapper = await mountDrawer()

    await click(wrapper, 'relay-method-password')
    await wrapper.get('[data-testid="relay-connect-email"]').setValue('you@example.com')
    await wrapper.get('[data-testid="relay-connect-password"]').setValue('hunter2')
    await click(wrapper, 'relay-login')

    expect(mocks.Login).toHaveBeenCalledWith(URL, 'you@example.com', 'hunter2')
    expect(wrapper.get<HTMLInputElement>('[data-testid="relay-connect-password"]').element.value).toBe('')
  })

  it('connects with an API key, then clears the input', async () => {
    mocks.ConnectWithKey.mockResolvedValue(INSTANCE)
    const wrapper = await mountDrawer()

    await click(wrapper, 'relay-method-key')
    expect(wrapper.get('[data-testid="relay-connect-help"]').text()).toContain('hive-relay tokens create')
    await wrapper.get('[data-testid="relay-connect-token"]').setValue('rk_key')
    await click(wrapper, 'relay-connect-submit')

    expect(mocks.ConnectWithKey).toHaveBeenCalledWith(URL, 'rk_key')
    expect(wrapper.get<HTMLInputElement>('[data-testid="relay-connect-token"]').element.value).toBe('')
  })

  it('surfaces a rejected key', async () => {
    mocks.ConnectWithKey.mockRejectedValue(new Error('relay rejected the key'))
    const wrapper = await mountDrawer()

    await click(wrapper, 'relay-method-key')
    await wrapper.get('[data-testid="relay-connect-token"]').setValue('bad')
    await click(wrapper, 'relay-connect-submit')

    expect(wrapper.get('[data-testid="relay-error"]').text()).toContain('rejected')
  })

  it('surfaces an unreachable relay', async () => {
    mocks.Methods.mockRejectedValue(new Error('relay: unreachable'))
    const wrapper = await mountDrawer()

    expect(wrapper.get('[data-testid="relay-error"]').text()).toContain('unreachable')
    expect(has(wrapper, 'relay-check')).toBe(true)
    expect(has(wrapper, 'relay-connect-email')).toBe(false)
  })

  it('lists a connected account and disconnects it', async () => {
    connectedAccounts(ACCOUNT)
    mocks.Disconnect.mockResolvedValue(undefined)
    const wrapper = await mountDrawer()

    expect(has(wrapper, `relay-connected-${ACCOUNT}`)).toBe(true)
    await click(wrapper, `relay-disconnect-${ACCOUNT}`)

    expect(mocks.Disconnect).toHaveBeenCalledWith(ACCOUNT)
  })

  it('links to the docs', async () => {
    const wrapper = await mountDrawer()
    await click(wrapper, 'relay-connect-docs')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://hivedesktop.com/desktop/inbox/sources/#hive-relay')
  })
})
