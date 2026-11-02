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
  node: 'sources.grafana_alerts',
  Editor,
  connect,
  provider: 'grafana',
  accounts: ['host-1', 'host-2'],
  config: (credential) => ({ credential, matchers: [] }),
  emptyMessage: 'No Grafana stack is connected',
})

describe('sources.grafana_alerts editor', () => {
  it('edits matchers one per line', async () => {
    const config: Config = { credential: 'grafana/host-1', matchers: ['squad=platform'] }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.grafana_alerts-editor-matchers"]')
    expect((field.element as HTMLTextAreaElement).value).toBe('squad=platform')

    await field.setValue('squad=platform\nseverity=~critical|warning')
    expect(wrapper.emitted('update:config')).toEqual([
      [
        {
          credential: 'grafana/host-1',
          matchers: ['squad=platform', 'severity=~critical|warning'],
        },
      ],
    ])
    wrapper.unmount()
  })
})

describe('sources.grafana_alerts validate', () => {
  it('requires a connected stack', () => {
    expect(validate(defaults)).toEqual(['a source needs a connected Grafana stack'])
  })

  it('rejects a credential that is not a grafana ref', () => {
    expect(validate({ credential: 'github/octocat', matchers: [] })).toEqual([
      'credential must look like "grafana/<account>"',
    ])
  })

  it('accepts a grafana ref', () => {
    expect(validate({ credential: 'grafana/host-1', matchers: [] })).toEqual([])
  })

  it('accepts every matcher operator', () => {
    const matchers = ['a=b', 'c!=d', 'e=~f|g', 'h!~i']
    expect(validate({ credential: 'grafana/host-1', matchers })).toEqual([])
  })

  // The leftmost operator separates the label from the value, so an operator
  // inside the value is not a second matcher.
  it('accepts an operator inside a matcher value', () => {
    expect(validate({ credential: 'grafana/host-1', matchers: ['path=/a!=b'] })).toEqual([])
  })

  it('rejects a matcher with no operator or no label', () => {
    expect(validate({ credential: 'grafana/host-1', matchers: ['squad'] })).toEqual([
      'matcher "squad" needs one of =, !=, =~, !~',
    ])
    expect(validate({ credential: 'grafana/host-1', matchers: ['=platform'] })).toEqual([
      'matcher "=platform" has no label name',
    ])
  })
})
