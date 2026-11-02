import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
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
  type: 'sources.github',
  title: 'GitHub source',
  stability: 'stable',
  mode: 'pull',
  provider: 'github',
  envOverride: false,
})

beforeEach(() => connect())

describeCredentialPicker({
  node: 'sources.github',
  Editor,
  connect,
  provider: 'github',
  accounts: ['octocat', 'hubot'],
  config: (credential) => ({ credential, kind: 'notifications' }),
  emptyMessage: 'No GitHub account is connected',
})

describe('sources.github editor', () => {
  it('renders the current kind and query', () => {
    const config: Config = { credential: 'github/octocat', kind: 'search', query: 'is:open is:pr' }
    const wrapper = mount(Editor, { props: { config } })
    expect(wrapper.get('[data-testid="sources.github-editor-kind"]').text()).toContain('Search')
    expect(wrapper.get<HTMLInputElement>('[data-testid="sources.github-editor-query"]').element.value).toBe(
      'is:open is:pr',
    )
  })

  it('emits an immutable update:config on query edit, without mutating the config prop', async () => {
    const config: Config = { credential: 'github/octocat', kind: 'search', query: '' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="sources.github-editor-query"]').element
    input.value = 'is:open'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.query).toBe('') // prop untouched
    expect(wrapper.emitted('update:config')).toEqual([
      [{ credential: 'github/octocat', kind: 'search', query: 'is:open' }],
    ])
  })

  it('hides the query field and clears it when switching to notifications', async () => {
    const config: Config = { credential: 'github/octocat', kind: 'search', query: 'is:open' }
    const wrapper = mount(Editor, { props: { config } })

    await chooseOption(wrapper, 'sources.github-editor-kind', 'notifications')

    expect(wrapper.emitted('update:config')).toEqual([
      [{ credential: 'github/octocat', kind: 'notifications', query: '' }],
    ])
    wrapper.unmount()
  })
})

describe('sources.github validate', () => {
  it('requires a query for search sources', () => {
    expect(validate({ credential: 'github/octocat', kind: 'search', query: '  ' })).toEqual([
      'a search source requires a query',
    ])
    expect(validate({ credential: 'github/octocat', kind: 'search', query: 'is:open' })).toEqual([])
  })

  // A freshly dropped node names no account, exactly as it carries no query.
  // Both are things the user supplies, so both are reported at once rather
  // than one after the other.
  it('reports the unset account alongside the unset query', () => {
    expect(validate(defaults)).toEqual([
      'a source needs a connected GitHub account',
      'a search source requires a query',
    ])
  })

  it('rejects a credential that is not a github ref', () => {
    expect(validate({ credential: 'octocat', kind: 'notifications' })).toEqual([
      'credential must look like "github/<login>"',
    ])
    expect(validate({ credential: 'grafana/prod', kind: 'notifications' })).toEqual([
      'credential must look like "github/<login>"',
    ])
  })

  it('rejects a query on notifications sources', () => {
    expect(validate({ credential: 'github/octocat', kind: 'notifications' })).toEqual([])
    expect(validate({ credential: 'github/octocat', kind: 'notifications', query: 'is:open' })).toEqual([
      'a notifications source takes no query',
    ])
  })

  it('enforces per-kind limit caps', () => {
    expect(validate({ credential: 'github/octocat', kind: 'search', query: 'x', limit: 101 })).toEqual([
      'search limit caps at 100',
    ])
    expect(validate({ credential: 'github/octocat', kind: 'notifications', limit: 51 })).toEqual([
      'notifications limit caps at 50',
    ])
  })
})
