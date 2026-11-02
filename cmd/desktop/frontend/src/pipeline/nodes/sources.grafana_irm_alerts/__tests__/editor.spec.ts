import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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

function config(overrides: Partial<Config> = {}): Config {
  return { ...defaults, ...overrides }
}

const connect = connectAccounts(mocks.List, {
  key: 'grafana',
  title: 'Grafana',
  stability: 'stable',
  provider: 'grafana',
  types: ['sources.grafana_alerts', 'sources.grafana_irm_alerts', 'sources.grafana_metrics'],
  envOverride: false,
})

beforeEach(() => connect())

describeCredentialPicker({
  node: 'sources.grafana_irm_alerts',
  Editor,
  connect,
  provider: 'grafana',
  accounts: ['host-1', 'host-2'],
  config: (credential) => ({ ...defaults, credential }),
  emptyMessage: 'No Grafana stack is connected',
})

describe('sources.grafana_irm_alerts editor', () => {
  it('emits the integration scope without disturbing the other fields', async () => {
    const wrapper = mount(Editor, { props: { config: config({ credential: 'grafana/host-1', team: 'T1' }) } })
    await flushPromises()

    await wrapper.get('[data-testid="sources.grafana_irm_alerts-editor-integration"]').setValue('CFRPV98RPR1U8')
    expect(wrapper.emitted('update:config')).toEqual([
      [
        {
          credential: 'grafana/host-1',
          integration: 'CFRPV98RPR1U8',
          team: 'T1',
        },
      ],
    ])
    wrapper.unmount()
  })
})

describe('sources.grafana_irm_alerts validate', () => {
  it('requires a connected stack', () => {
    expect(validate(defaults)).toEqual(['a source needs a connected Grafana stack'])
  })

  it('rejects a credential that is not a grafana ref', () => {
    expect(validate(config({ credential: 'github/octocat' }))).toEqual([
      'credential must look like "grafana/<account>"',
    ])
  })

  // Scope is optional: an unscoped node is a valid whole-stack on-call feed.
  it('accepts a grafana ref with no scope', () => {
    expect(validate(config({ credential: 'grafana/host-1' }))).toEqual([])
  })
})
