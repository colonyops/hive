import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate, type Config } from '../config'
import { chooseOption } from '../../../../test-utils/select'

const mocks = vi.hoisted(() => ({ List: vi.fn(), On: vi.fn() }))

vi.mock(
  '../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice',
  () => ({
    List: mocks.List,
  }),
)
vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
}))

function connectedStacks(...accounts: string[]) {
  mocks.List.mockResolvedValue([
    {
      key: 'grafana',
      title: 'Grafana',
      stability: 'stable',
      provider: 'grafana',
      types: ['sources.grafana_alerts', 'sources.grafana_metrics'],
      accounts,
      envOverride: false,
    },
  ])
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  connectedStacks()
})

describe('sources.grafana_metrics editor', () => {
  it('renders the current datasource and query', () => {
    const config: Config = { credential: 'grafana/host-1', datasource_uid: 'ds-uid', expr: 'up' }
    const wrapper = mount(Editor, { props: { config } })
    expect(
      wrapper.get<HTMLInputElement>('[data-testid="sources.grafana_metrics-editor-datasource"]').element.value,
    ).toBe('ds-uid')
    expect(wrapper.get<HTMLInputElement>('[data-testid="sources.grafana_metrics-editor-expr"]').element.value).toBe(
      'up',
    )
  })

  it('emits an immutable update:config on query edit, without mutating the config prop', async () => {
    const config: Config = { credential: 'grafana/host-1', datasource_uid: 'ds', expr: '' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="sources.grafana_metrics-editor-expr"]').element
    input.value = 'up'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.expr).toBe('') // prop untouched
    expect(wrapper.emitted('update:config')).toEqual([
      [{ credential: 'grafana/host-1', datasource_uid: 'ds', expr: 'up' }],
    ])
  })

  it('offers the connected stacks as options', async () => {
    connectedStacks('host-1', 'host-2')
    const config: Config = { credential: 'grafana/host-1', datasource_uid: 'ds', expr: 'up' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.grafana_metrics-editor-credential"]')
    expect(field.text()).toContain('grafana/host-1')
    await chooseOption(wrapper, 'sources.grafana_metrics-editor-credential', 'grafana/host-2')
    expect(wrapper.emitted('update:config')).toEqual([
      [{ credential: 'grafana/host-2', datasource_uid: 'ds', expr: 'up' }],
    ])
    wrapper.unmount()
  })

  it('keeps a stack that is no longer connected selectable, and says so', async () => {
    connectedStacks('host-2')
    const config: Config = { credential: 'grafana/host-1', datasource_uid: 'ds', expr: 'up' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    expect(wrapper.get('[data-testid="sources.grafana_metrics-editor-credential"]').text()).toContain('not connected')
    wrapper.unmount()
  })

  it('falls back to a text input when no stack is connected', async () => {
    const config: Config = { credential: '', datasource_uid: '', expr: '' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.grafana_metrics-editor-credential"]')
    expect(field.element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No Grafana stack is connected')
    wrapper.unmount()
  })
})

describe('sources.grafana_metrics validate', () => {
  it('reports every unset field at once', () => {
    expect(validate(defaults)).toEqual([
      'a source needs a connected Grafana stack',
      'a datasource uid is required',
      'a PromQL query is required',
    ])
  })

  it('rejects a credential that is not a grafana ref', () => {
    expect(validate({ credential: 'github/octocat', datasource_uid: 'ds', expr: 'up' })).toEqual([
      'credential must look like "grafana/<account>"',
    ])
  })

  it('accepts a complete config', () => {
    expect(validate({ credential: 'grafana/host-1', datasource_uid: 'ds', expr: 'up' })).toEqual([])
  })
})
