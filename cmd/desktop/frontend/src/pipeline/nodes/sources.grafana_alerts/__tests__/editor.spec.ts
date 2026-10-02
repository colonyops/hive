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
      types: ['sources.grafana_alerts', 'sources.grafana_irm_alerts', 'sources.grafana_metrics'],
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

describe('sources.grafana_alerts editor', () => {
  it('offers the connected stacks and emits the choice', async () => {
    connectedStacks('host-1', 'host-2')
    const config: Config = { credential: 'grafana/host-1', matchers: [] }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.grafana_alerts-editor-credential"]')
    expect(field.text()).toContain('grafana/host-1')
    await chooseOption(wrapper, 'sources.grafana_alerts-editor-credential', 'grafana/host-2')
    expect(wrapper.emitted('update:config')).toEqual([[{ credential: 'grafana/host-2', matchers: [] }]])
    wrapper.unmount()
  })

  it('falls back to a text input when no stack is connected', async () => {
    const config: Config = { credential: '', matchers: [] }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.grafana_alerts-editor-credential"]')
    expect(field.element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No Grafana stack is connected')
    wrapper.unmount()
  })

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
