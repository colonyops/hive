import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate, type Config } from '../config'
import { chooseOption } from '../../../../test-utils/select'

const mocks = vi.hoisted(() => ({ List: vi.fn(), On: vi.fn() }))

vi.mock('../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice', () => ({
  List: mocks.List,
}))
vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
}))

function connectedProjects(...accounts: string[]) {
  mocks.List.mockResolvedValue([
    { key: 'posthog', title: 'PostHog', stability: 'stable', provider: 'posthog', types: ['sources.posthog_errors', 'sources.posthog_alerts'], accounts, envOverride: false },
  ])
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  connectedProjects()
})

describe('sources.posthog_errors editor', () => {
  it('offers the connected projects and emits the choice', async () => {
    connectedProjects('us.posthog.com-1', 'us.posthog.com-2')
    const config: Config = { ...defaults, credential: 'posthog/us.posthog.com-1' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.posthog_errors-editor-credential"]')
    expect(field.text()).toContain('posthog/us.posthog.com-1')

    await chooseOption(wrapper, 'sources.posthog_errors-editor-credential', 'posthog/us.posthog.com-2')
    expect(wrapper.emitted('update:config')).toEqual([[{ ...config, credential: 'posthog/us.posthog.com-2' }]])
    wrapper.unmount()
  })

  it('falls back to a text input when no project is connected', async () => {
    const wrapper = mount(Editor, { props: { config: { ...defaults } } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.posthog_errors-editor-credential"]')
    expect(field.element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No PostHog project is connected')
    wrapper.unmount()
  })

  // A since-disconnected credential must stay selectable; dropping it would
  // silently rewrite the node's config on the next edit.
  it('keeps a disconnected credential in the list', async () => {
    connectedProjects('us.posthog.com-2')
    const config: Config = { ...defaults, credential: 'posthog/us.posthog.com-1' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    expect(wrapper.get('[data-testid="sources.posthog_errors-editor-credential"]').text()).toContain('not connected')
    wrapper.unmount()
  })

  it('emits the scoping fields', async () => {
    connectedProjects('us.posthog.com-1')
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
