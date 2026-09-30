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

describe('sources.posthog_alerts editor', () => {
  it('offers the connected projects and emits the choice', async () => {
    connectedProjects('us.posthog.com-1', 'us.posthog.com-2')
    const config: Config = { ...defaults, credential: 'posthog/us.posthog.com-1' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.posthog_alerts-editor-credential"]')
    expect(field.text()).toContain('posthog/us.posthog.com-1')

    await chooseOption(wrapper, 'sources.posthog_alerts-editor-credential', 'posthog/us.posthog.com-2')
    expect(wrapper.emitted('update:config')).toEqual([[{ ...config, credential: 'posthog/us.posthog.com-2' }]])
    wrapper.unmount()
  })

  it('falls back to a text input when no project is connected', async () => {
    const wrapper = mount(Editor, { props: { config: { ...defaults } } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.posthog_alerts-editor-credential"]')
    expect(field.element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No PostHog project is connected')
    wrapper.unmount()
  })

  // Firing-only is off by default on purpose: emitting every alert is what
  // lets one that stops firing update the item that was already there.
  it('defaults to emitting every alert', () => {
    expect(defaults.firing_only).toBe(false)
  })
})

describe('sources.posthog_alerts validate', () => {
  it('requires a connected project', () => {
    expect(validate(defaults)).toEqual(['a source needs a connected PostHog project'])
  })

  it('rejects a credential that is not a posthog ref', () => {
    expect(validate({ credential: 'grafana/host-1' })).toEqual(['credential must look like "posthog/<account>"'])
  })

  it('accepts a posthog ref', () => {
    expect(validate({ credential: 'posthog/us.posthog.com-1' })).toEqual([])
  })
})
