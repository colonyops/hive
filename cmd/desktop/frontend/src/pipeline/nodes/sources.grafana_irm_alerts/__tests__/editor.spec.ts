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

function connectedStacks(...accounts: string[]) {
  mocks.List.mockResolvedValue([
    { key: 'grafana', title: 'Grafana', stability: 'stable', provider: 'grafana', types: ['sources.grafana_alerts', 'sources.grafana_irm_alerts', 'sources.grafana_metrics'], accounts, envOverride: false },
  ])
}

function config(overrides: Partial<Config> = {}): Config {
  return { ...defaults, ...overrides }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  connectedStacks()
})

describe('sources.grafana_irm_alerts editor', () => {
  it('offers the connected stacks and emits the choice', async () => {
    connectedStacks('host-1', 'host-2')
    const wrapper = mount(Editor, { props: { config: config({ credential: 'grafana/host-1' }) } })
    await flushPromises()

    await chooseOption(wrapper, 'sources.grafana_irm_alerts-editor-credential', 'grafana/host-2')
    expect(wrapper.emitted('update:config')).toEqual([[{ credential: 'grafana/host-2', integration: '', team: '' }]])
    wrapper.unmount()
  })

  it('falls back to a text input when no stack is connected', async () => {
    const wrapper = mount(Editor, { props: { config: config() } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.grafana_irm_alerts-editor-credential"]')
    expect(field.element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No Grafana stack is connected')
    wrapper.unmount()
  })

  it('emits the integration scope without disturbing the other fields', async () => {
    const wrapper = mount(Editor, { props: { config: config({ credential: 'grafana/host-1', team: 'T1' }) } })
    await flushPromises()

    await wrapper.get('[data-testid="sources.grafana_irm_alerts-editor-integration"]').setValue('CFRPV98RPR1U8')
    expect(wrapper.emitted('update:config')).toEqual([[{
      credential: 'grafana/host-1',
      integration: 'CFRPV98RPR1U8',
      team: 'T1',
    }]])
    wrapper.unmount()
  })
})

describe('sources.grafana_irm_alerts validate', () => {
  it('requires a connected stack', () => {
    expect(validate(defaults)).toEqual(['a source needs a connected Grafana stack'])
  })

  it('rejects a credential that is not a grafana ref', () => {
    expect(validate(config({ credential: 'github/octocat' }))).toEqual(['credential must look like "grafana/<account>"'])
  })

  // Scope is optional: an unscoped node is a valid whole-stack on-call feed.
  it('accepts a grafana ref with no scope', () => {
    expect(validate(config({ credential: 'grafana/host-1' }))).toEqual([])
  })
})
