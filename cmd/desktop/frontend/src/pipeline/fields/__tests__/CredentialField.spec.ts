import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CredentialField from '../CredentialField.vue'
import { chooseOption } from '../../../test-utils/select'

const mocks = vi.hoisted(() => ({ List: vi.fn(), On: vi.fn() }))

vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice',
  () => ({
    List: mocks.List,
  }),
)
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function connected(...accounts: string[]) {
  mocks.List.mockResolvedValue([
    {
      key: 'posthog',
      title: 'PostHog',
      stability: 'stable',
      provider: 'posthog',
      types: [],
      accounts,
      envOverride: false,
    },
  ])
}

async function mountField(modelValue?: string) {
  const wrapper = mount(CredentialField, { props: { provider: 'posthog', modelValue, testid: 'cred' } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  connected()
})

describe('CredentialField', () => {
  it('offers the connected credentials and emits the chosen ref', async () => {
    connected('a', 'b')
    const wrapper = await mountField('posthog/a')
    expect(wrapper.text()).toContain('The connected PostHog project to fetch as.')
    await chooseOption(wrapper, 'cred', 'posthog/b')
    expect(wrapper.emitted('update:modelValue')).toEqual([['posthog/b']])
    wrapper.unmount()
  })

  it('keeps a disconnected credential selectable and marks it', async () => {
    connected('b')
    const wrapper = await mountField('posthog/a')
    expect(wrapper.get('[data-testid="cred"]').text()).toContain('posthog/a — not connected')
    wrapper.unmount()
  })

  it('falls back to a text input when nothing is connected', async () => {
    const wrapper = await mountField()
    expect(wrapper.get('[data-testid="cred"]').element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No PostHog project is connected')
    wrapper.unmount()
  })
})
