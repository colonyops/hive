import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate, type Config } from '../config'
import { chooseOption } from '@hive/ui/test-utils/select'
import { connectAccounts, describeCredentialPicker } from '../../../__tests__/credentialPicker'

const mocks = vi.hoisted(() => ({ List: vi.fn() }))

vi.mock(
  '../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice',
  () => ({
    List: mocks.List,
  }),
)

const connect = connectAccounts(mocks.List, {
  key: 'posthog',
  title: 'PostHog',
  stability: 'stable',
  provider: 'posthog',
  types: ['sources.posthog_errors', 'sources.posthog_alerts'],
  envOverride: false,
})

beforeEach(() => connect())

describeCredentialPicker({
  node: 'sources.posthog_errors',
  Editor,
  connect,
  provider: 'posthog',
  accounts: ['us.posthog.com-1', 'us.posthog.com-2'],
  config: (credential) => ({ ...defaults, credential }),
  emptyMessage: 'No PostHog project is connected',
})

describe('sources.posthog_errors editor', () => {
  it('emits the scoping fields', async () => {
    connect('us.posthog.com-1')
    const config: Config = { ...defaults, credential: 'posthog/us.posthog.com-1' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    await chooseOption(wrapper, 'sources.posthog_errors-editor-status', 'all')
    expect(wrapper.emitted('update:config')?.[0]).toEqual([{ ...config, status: 'all' }])

    await chooseOption(wrapper, 'sources.posthog_errors-editor-order-by', 'occurrences')
    expect(wrapper.emitted('update:config')?.[1]).toEqual([{ ...config, order_by: 'occurrences' }])

    const limit = wrapper.get('[data-testid="sources.posthog_errors-editor-limit"]')
    await limit.setValue('50')
    expect(wrapper.emitted('update:config')?.[2]).toEqual([{ ...config, limit: 50 }])
    wrapper.unmount()
  })
})

describe('sources.posthog_errors validate', () => {
  it('requires a connected project', () => {
    expect(validate(defaults)).toEqual(['a source needs a connected PostHog project'])
  })

  it('rejects a credential that is not a posthog ref', () => {
    expect(validate({ credential: 'grafana/host-1' })).toEqual(['credential must look like "posthog/<account>"'])
  })

  it('accepts a posthog ref', () => {
    expect(validate({ credential: 'posthog/us.posthog.com-1' })).toEqual([])
  })

  it('rejects a limit past the query page cap', () => {
    expect(validate({ credential: 'posthog/us.posthog.com-1', limit: 101 })).toEqual(['limit caps at 100'])
    expect(validate({ credential: 'posthog/us.posthog.com-1', limit: -1 })).toEqual(['limit must not be negative'])
  })

  it('accepts the defaults once a credential is filled in', () => {
    expect(validate({ ...defaults, credential: 'posthog/us.posthog.com-1' })).toEqual([])
  })
})
