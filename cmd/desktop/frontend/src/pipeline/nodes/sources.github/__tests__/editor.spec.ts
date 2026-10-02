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

function connectedAccounts(...accounts: string[]) {
  mocks.List.mockResolvedValue([
    {
      type: 'sources.github',
      title: 'GitHub source',
      stability: 'stable',
      mode: 'pull',
      provider: 'github',
      accounts,
      envOverride: false,
    },
  ])
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  connectedAccounts()
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

  // The field used to be a free-text box the user had to type a ref into.
  // It offers the accounts the credential store actually holds instead, which
  // is what makes a typo'd ref unrepresentable rather than merely invalid.
  it('offers the connected accounts as options', async () => {
    connectedAccounts('octocat', 'hubot')
    const config: Config = { credential: 'github/octocat', kind: 'notifications' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.github-editor-credential"]')
    expect(field.text()).toContain('github/octocat')
    await chooseOption(wrapper, 'sources.github-editor-credential', 'github/hubot')
    expect(wrapper.emitted('update:config')).toEqual([[{ credential: 'github/hubot', kind: 'notifications' }]])
    wrapper.unmount()
  })

  // Dropping a disconnected account from the list would silently rewrite the
  // node's config on the next edit of any other field.
  it('keeps an account that is no longer connected selectable, and says so', async () => {
    connectedAccounts('hubot')
    const config: Config = { credential: 'github/octocat', kind: 'notifications' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.github-editor-credential"]')
    expect(field.text()).toContain('not connected')
    wrapper.unmount()
  })

  // An empty dropdown is a dead end: with nothing connected and nothing set,
  // the field stays typeable so a flow can still be authored ahead of the
  // account existing.
  it('falls back to a text input when no account is connected', async () => {
    const config: Config = { credential: '', kind: 'notifications' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    const field = wrapper.get('[data-testid="sources.github-editor-credential"]')
    expect(field.element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No GitHub account is connected')
    wrapper.unmount()
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
