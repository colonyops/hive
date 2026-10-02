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
      type: 'sources.gitea',
      title: 'Gitea source',
      stability: 'stable',
      mode: 'pull',
      provider: 'gitea',
      accounts,
      envOverride: false,
    },
  ])
}

const CREDENTIAL = 'gitea/git.example.com-octocat'

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  connectedAccounts()
})

describe('sources.gitea editor', () => {
  it('renders the search filters for a search source', () => {
    const config: Config = { credential: CREDENTIAL, kind: 'search', items: 'pulls', state: 'open', owner: 'acme' }
    const wrapper = mount(Editor, { props: { config } })

    expect(wrapper.get('[data-testid="sources.gitea-editor-items"]').text()).toContain('Pull requests only')
    expect(wrapper.get<HTMLInputElement>('[data-testid="sources.gitea-editor-owner"]').element.value).toBe('acme')
  })

  // The inbox cannot be filtered server-side, so the fields are not merely
  // hidden — a node that looks filtered and returns everything is worse than
  // one that says the filters do not apply.
  it('hides the search filters for a notifications source', () => {
    const config: Config = { credential: CREDENTIAL, kind: 'notifications' }
    const wrapper = mount(Editor, { props: { config } })

    expect(wrapper.find('[data-testid="sources.gitea-editor-items"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sources.gitea-editor-owner"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sources.gitea-editor-limit"]').exists()).toBe(true)
  })

  // Go rejects a notifications source still carrying filters, so leaving them
  // set would produce a node that cannot be saved.
  it('clears the search filters when switching to notifications', async () => {
    const config: Config = {
      credential: CREDENTIAL,
      kind: 'search',
      items: 'pulls',
      state: 'all',
      involving: ['assigned'],
      owner: 'acme',
      labels: ['bug'],
      text: 'x',
    }
    const wrapper = mount(Editor, { props: { config } })

    await chooseOption(wrapper, 'sources.gitea-editor-kind', 'notifications')

    expect(wrapper.emitted('update:config')).toEqual([
      [
        {
          credential: CREDENTIAL,
          kind: 'notifications',
          items: undefined,
          state: undefined,
          involving: undefined,
          owner: undefined,
          labels: undefined,
          text: undefined,
        },
      ],
    ])
    wrapper.unmount()
  })

  it('emits an immutable update:config on an owner edit, without mutating the prop', async () => {
    const config: Config = { credential: CREDENTIAL, kind: 'search' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="sources.gitea-editor-owner"]').element
    input.value = 'acme'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.owner).toBeUndefined() // prop untouched
    expect(wrapper.emitted('update:config')).toEqual([[{ credential: CREDENTIAL, kind: 'search', owner: 'acme' }]])
  })

  it('toggles an involvement into the list in the declared order', async () => {
    const config: Config = { credential: CREDENTIAL, kind: 'search', involving: ['assigned'] }
    const wrapper = mount(Editor, { props: { config } })

    await wrapper.get('[data-testid="sources.gitea-editor-involving-created"]').setValue(true)

    expect(wrapper.emitted('update:config')).toEqual([
      [
        {
          credential: CREDENTIAL,
          kind: 'search',
          involving: ['created', 'assigned'],
        },
      ],
    ])
  })

  it('drops an involvement when it is unticked', async () => {
    const config: Config = { credential: CREDENTIAL, kind: 'search', involving: ['created', 'assigned'] }
    const wrapper = mount(Editor, { props: { config } })

    await wrapper.get('[data-testid="sources.gitea-editor-involving-created"]').setValue(false)

    expect(wrapper.emitted('update:config')).toEqual([
      [
        {
          credential: CREDENTIAL,
          kind: 'search',
          involving: ['assigned'],
        },
      ],
    ])
  })

  // Gitea intersects its involvement parameters, so a union costs one request
  // each. That is worth stating where it is chosen, not only in the node docs.
  it('says how many requests a union of involvements costs', async () => {
    const config: Config = { credential: CREDENTIAL, kind: 'search', involving: ['created', 'assigned'] }
    const wrapper = mount(Editor, { props: { config } })

    expect(wrapper.get('[data-testid="sources.gitea-editor-involving-hint"]').text()).toContain('2 requests per poll')
  })

  it('offers the connected accounts as options', async () => {
    connectedAccounts('git.example.com-octocat', 'git.example.com-hubot')
    const config: Config = { credential: CREDENTIAL, kind: 'notifications' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    expect(wrapper.get('[data-testid="sources.gitea-editor-credential"]').text()).toContain(CREDENTIAL)
    await chooseOption(wrapper, 'sources.gitea-editor-credential', 'gitea/git.example.com-hubot')
    expect(wrapper.emitted('update:config')).toEqual([
      [{ credential: 'gitea/git.example.com-hubot', kind: 'notifications' }],
    ])
    wrapper.unmount()
  })

  // Dropping a disconnected account from the list would silently rewrite the
  // node's config on the next edit of any other field.
  it('keeps an account that is no longer connected selectable, and says so', async () => {
    connectedAccounts('git.example.com-hubot')
    const config: Config = { credential: CREDENTIAL, kind: 'notifications' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    expect(wrapper.get('[data-testid="sources.gitea-editor-credential"]').text()).toContain('not connected')
    wrapper.unmount()
  })

  it('falls back to a text input when no instance is connected', async () => {
    const config: Config = { credential: '', kind: 'notifications' }
    const wrapper = mount(Editor, { props: { config } })
    await flushPromises()

    expect(wrapper.get('[data-testid="sources.gitea-editor-credential"]').element.tagName).toBe('INPUT')
    expect(wrapper.text()).toContain('No Gitea instance is connected')
    wrapper.unmount()
  })
})

describe('sources.gitea validate', () => {
  it('accepts a bare search source — every filter is optional', () => {
    expect(validate({ credential: CREDENTIAL, kind: 'search' })).toEqual([])
  })

  it('reports only the unset account on a fresh node', () => {
    expect(validate(defaults)).toEqual(['a source needs a connected Gitea account'])
  })

  it('rejects a credential that is not a gitea ref', () => {
    expect(validate({ credential: 'github/octocat', kind: 'search' })).toEqual([
      'credential must look like "gitea/<host>-<login>"',
    ])
  })

  it('rejects search filters on a notifications source', () => {
    expect(validate({ credential: CREDENTIAL, kind: 'notifications', state: 'open', owner: 'acme' })).toEqual([
      'a notifications source takes no search filters (remove state, owner)',
    ])
  })

  // An empty involving list is what a freshly dropped node carries, and it
  // filters nothing — so it must not read as a filter that has been set.
  it('does not count an empty involving list as a search filter', () => {
    expect(validate({ credential: CREDENTIAL, kind: 'notifications', involving: [] })).toEqual([])
  })

  it('enforces the per-kind limits', () => {
    expect(validate({ credential: CREDENTIAL, kind: 'search', limit: 101 })).toEqual(['search limit caps at 100'])
    expect(validate({ credential: CREDENTIAL, kind: 'notifications', limit: 51 })).toEqual([
      'notifications limit caps at 50',
    ])
    expect(validate({ credential: CREDENTIAL, kind: 'search', limit: -1 })).toEqual(['limit must not be negative'])
  })

  it('rejects values Gitea would silently ignore', () => {
    expect(validate({ credential: CREDENTIAL, kind: 'search', items: 'prs' as never })).toEqual([
      'items must be one of all, issues, pulls',
    ])
    expect(validate({ credential: CREDENTIAL, kind: 'search', state: 'merged' as never })).toEqual([
      'state must be one of open, closed, all',
    ])
  })
})
