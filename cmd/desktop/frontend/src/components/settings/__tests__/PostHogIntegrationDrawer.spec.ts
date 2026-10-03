import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PostHogIntegrationDrawer from '../PostHogIntegrationDrawer.vue'

const mocks = vi.hoisted(() => ({
  Connect: vi.fn(),
  Disconnect: vi.fn(),
  Projects: vi.fn(),
  List: vi.fn(),
  On: vi.fn(),
  OpenURL: vi.fn(),
}))

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/posthogservice', () => ({
  Connect: mocks.Connect,
  Disconnect: mocks.Disconnect,
  Projects: mocks.Projects,
}))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice',
  () => ({ List: mocks.List }),
)
vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Browser: { OpenURL: mocks.OpenURL },
}))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  mocks.List.mockResolvedValue([])
  mocks.Projects.mockResolvedValue([
    { account: '', url: 'https://us.posthog.com', id: 1, name: 'Dev' },
    { account: '', url: 'https://us.posthog.com', id: 2, name: 'Prod' },
  ])
})

async function mountWithProjects() {
  const wrapper = mount(PostHogIntegrationDrawer, { global: { stubs: { Teleport: true } } })
  await flushPromises()
  await wrapper.get('[data-testid="posthog-connect-token"]').setValue('phx_key')
  await wrapper.get('[data-testid="posthog-connect-load"]').trigger('click')
  await flushPromises()
  return wrapper
}

const input = (wrapper: Awaited<ReturnType<typeof mountWithProjects>>, testid: string) =>
  wrapper.get<HTMLInputElement>(`[data-testid="${testid}"]`).element

describe('PostHogIntegrationDrawer', () => {
  it('starts on the US cloud host and asks for a key before it can look up projects', async () => {
    const wrapper = mount(PostHogIntegrationDrawer, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    expect(input(wrapper, 'posthog-connect-url').value).toBe('https://us.posthog.com')
    expect(wrapper.get('[data-testid="posthog-connect-load"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="posthog-connect-submit"]').exists()).toBe(false)
  })

  it('lists the projects the key reaches, locks the key, and connects the first one', async () => {
    mocks.Connect.mockResolvedValue({ account: 'us.posthog.com-1' })
    const wrapper = await mountWithProjects()

    expect(mocks.Projects).toHaveBeenCalledWith('https://us.posthog.com', 'phx_key')
    expect(input(wrapper, 'posthog-connect-token').disabled).toBe(true)

    await wrapper.get('[data-testid="posthog-connect-submit"]').trigger('click')
    await flushPromises()

    expect(mocks.Connect).toHaveBeenCalledWith('https://us.posthog.com', 'phx_key', 1)
    expect(wrapper.find('[data-testid="posthog-connect-load"]').exists()).toBe(true)
    // The key stays for the next project.
    expect(input(wrapper, 'posthog-connect-token').value).toBe('phx_key')
    expect(input(wrapper, 'posthog-connect-token').disabled).toBe(false)
  })

  it('keeps the picker open with the error when the connect fails, and starts over on request', async () => {
    mocks.Connect.mockRejectedValue(new Error('project is gone'))
    const wrapper = await mountWithProjects()

    await wrapper.get('[data-testid="posthog-connect-submit"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="posthog-connect-error"]').text()).toContain('project is gone')

    await wrapper.get('[data-testid="posthog-connect-back"]').trigger('click')
    expect(wrapper.find('[data-testid="posthog-connect-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="posthog-connect-load"]').exists()).toBe(true)
  })

  it('reports a key PostHog rejects', async () => {
    mocks.Projects.mockRejectedValue({})
    const wrapper = await mountWithProjects()

    expect(wrapper.get('[data-testid="posthog-connect-error"]').text()).toBe('PostHog rejected the API key.')
    expect(wrapper.find('[data-testid="posthog-connect-submit"]').exists()).toBe(false)
  })
})
