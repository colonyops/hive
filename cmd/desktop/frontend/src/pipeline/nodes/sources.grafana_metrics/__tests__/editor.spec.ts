import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate, type Config } from '../config'
import { connectAccounts, describeCredentialPicker } from '../../../__tests__/credentialPicker'

const mocks = vi.hoisted(() => ({ List: vi.fn() }))

vi.mock(
  '../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice',
  () => ({
    List: mocks.List,
  }),
)

const connect = connectAccounts(mocks.List, {
  key: 'grafana',
  title: 'Grafana',
  stability: 'stable',
  provider: 'grafana',
  types: ['sources.grafana_alerts', 'sources.grafana_metrics'],
  envOverride: false,
})

beforeEach(() => connect())

describeCredentialPicker({
  node: 'sources.grafana_metrics',
  Editor,
  connect,
  provider: 'grafana',
  accounts: ['host-1', 'host-2'],
  config: (credential) => ({ credential, datasource_uid: 'ds', expr: 'up' }),
  emptyMessage: 'No Grafana stack is connected',
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
