import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Editor, { type RelayEditorClient } from '../editor.vue'
import { defaults, validate, type Config } from '../config'
import { chooseOption, openSelect } from '../../../../test-utils/select'
import type {
  RecentDelivery,
  SourceView,
} from '../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay/models'

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

const ACCOUNT = 'relay.hivedesktop.com-you@example.com'
const CREDENTIAL = `relay/${ACCOUNT}`
const URL = 'https://relay.hivedesktop.com/in/tok123'

function source(id: string, name: string, extra: Partial<SourceView> = {}): SourceView {
  return { id, name, has_secret: false, created_at: '2026-10-01T00:00:00Z', deliveries: 0, bytes: 0, ...extra }
}

function delivery(id: string, extra: Partial<RecentDelivery> = {}): RecentDelivery {
  return {
    id,
    receivedAt: '2026-10-11T09:30:00Z',
    method: 'POST',
    path: '/',
    contentType: 'application/json',
    body: '{"id":"build-42","title":"Build failed"}',
    dialect: 'generic',
    item: null,
    key: 'build-42',
    error: '',
    ...extra,
  }
}

function fakeClient(overrides: Partial<RelayEditorClient> = {}): RelayEditorClient {
  return {
    listSources: vi.fn(() => Promise.resolve([source('src-1', 'uptime'), source('src-2', 'ci')])),
    createSource: vi.fn((_account: string, name: string) =>
      Promise.resolve(source('src-new', name, { ingress_url: URL })),
    ),
    rotateSource: vi.fn(() => Promise.resolve(`${URL}-rotated`)),
    recentDeliveries: vi.fn(() => Promise.resolve<RecentDelivery[]>([])),
    setMarkImage: (data: string) => Promise.resolve({ hash: 'a'.repeat(32), image: `data:image/png;base64,${data}` }),
    markImage: () => Promise.resolve(undefined),
    ...overrides,
  }
}

type EditorWrapper = ReturnType<typeof mount<typeof Editor>>

let wrapper: EditorWrapper | undefined

function mountEditor(config: Config, client: RelayEditorClient = fakeClient()): EditorWrapper {
  wrapper = mount(Editor, { props: { config, client } })
  return wrapper
}

async function typeInto(w: EditorWrapper, testid: string, value: string) {
  const input = w.get<HTMLInputElement>(`[data-testid="${testid}"]`).element
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await w.vm.$nextTick()
}

function emitted(w: EditorWrapper): Config[] {
  return (w.emitted('update:config') ?? []).map((args) => args[0] as Config)
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  mocks.List.mockResolvedValue([])
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

const base: Config = { credential: CREDENTIAL, source: 'src-1', dialect: 'auto', interval: '1m' }

describe('sources.relay editor', () => {
  it('lists the account sources and keeps a missing one selectable', async () => {
    const client = fakeClient()
    const w = mountEditor({ ...base, source: 'gone' }, client)
    await flushPromises()

    expect(client.listSources).toHaveBeenCalledWith(ACCOUNT)
    const popover = await openSelect(w, 'sources.relay-editor-source')
    expect(popover.querySelector('[data-testid="sources.relay-editor-source-option-src-1"]')?.textContent).toContain(
      'uptime',
    )
    expect(popover.querySelector('[data-testid="sources.relay-editor-source-option-src-2"]')?.textContent).toContain(
      'ci',
    )
    expect(popover.querySelector('[data-testid="sources.relay-editor-source-option-gone"]')?.textContent).toContain(
      'not found',
    )
  })

  it('emits the picked source', async () => {
    const w = mountEditor(base)
    await flushPromises()
    await chooseOption(w, 'sources.relay-editor-source', 'src-2')
    expect(emitted(w)).toEqual([{ ...base, source: 'src-2' }])
  })

  it('does not load sources without an account', async () => {
    const client = fakeClient()
    mountEditor({ ...defaults }, client)
    await flushPromises()
    expect(client.listSources).not.toHaveBeenCalled()
  })

  it('creates a source, selects it, and shows its ingress URL once', async () => {
    const client = fakeClient()
    const w = mountEditor(base, client)
    await flushPromises()

    expect(w.find('[data-testid="sources.relay-editor-url"]').exists()).toBe(false)
    expect(w.find('[data-testid="sources.relay-editor-paste"]').exists()).toBe(false)

    await typeInto(w, 'sources.relay-editor-new-name', 'kuma')
    await typeInto(w, 'sources.relay-editor-new-secret', 's3cret')
    await w.get('[data-testid="sources.relay-editor-create"]').trigger('click')
    await flushPromises()

    expect(client.createSource).toHaveBeenCalledWith(ACCOUNT, 'kuma', 's3cret')
    expect(emitted(w)).toEqual([{ ...base, source: 'src-new' }])

    await w.setProps({ config: { ...base, source: 'src-new' } })
    expect(w.get('[data-testid="sources.relay-editor-url"]').text()).toBe(URL)
    expect(w.find('[data-testid="sources.relay-editor-copy-url"]').exists()).toBe(true)
    expect(w.get('[data-testid="sources.relay-editor-paste"]').text()).toContain(URL)
  })

  it('rotates the selected source and shows the new URL', async () => {
    const client = fakeClient()
    const w = mountEditor(base, client)
    await flushPromises()

    await w.get('[data-testid="sources.relay-editor-rotate"]').trigger('click')
    await flushPromises()

    expect(client.rotateSource).toHaveBeenCalledWith(ACCOUNT, 'src-1')
    expect(w.get('[data-testid="sources.relay-editor-url"]').text()).toBe(`${URL}-rotated`)
  })

  it('renders a backend failure', async () => {
    const client = fakeClient({ rotateSource: vi.fn(() => Promise.reject(new Error('relay unreachable'))) })
    const w = mountEditor(base, client)
    await flushPromises()

    await w.get('[data-testid="sources.relay-editor-rotate"]').trigger('click')
    await flushPromises()

    expect(w.get('[data-testid="sources.relay-editor-error"]').text()).toContain('relay unreachable')
  })

  it('emits a dialect change', async () => {
    const w = mountEditor(base)
    await flushPromises()
    await chooseOption(w, 'sources.relay-editor-dialect', 'generic')
    expect(emitted(w)).toEqual([{ ...base, dialect: 'generic' }])
  })

  it('emits a nested map on a pointer edit without mutating the prop', async () => {
    const config: Config = { ...base, map: { title: '/raw/body/name' } }
    const w = mountEditor(config)
    await flushPromises()

    await typeInto(w, 'sources.relay-editor-map-key', '/raw/body/monitor/id')

    expect(config.map).toEqual({ title: '/raw/body/name' })
    expect(emitted(w)).toEqual([{ ...base, map: { key: '/raw/body/monitor/id', title: '/raw/body/name' } }])
  })

  it('drops the map when its last field is cleared', async () => {
    const w = mountEditor({ ...base, map: { key: '/id' } })
    await flushPromises()
    await typeInto(w, 'sources.relay-editor-map-key', '')
    expect(emitted(w)).toEqual([{ ...base, map: undefined }])
  })

  it('adds, fills, and removes state rows', async () => {
    const w = mountEditor({ ...base, map: { state: '/raw/body/status', states: { '1': 'resolved' } } })
    await flushPromises()

    expect(w.get<HTMLInputElement>('[data-testid="sources.relay-editor-states-row-0-from"]').element.value).toBe('1')

    await w.get('[data-testid="sources.relay-editor-states-add"]').trigger('click')
    await typeInto(w, 'sources.relay-editor-states-row-1-from', '0')
    await typeInto(w, 'sources.relay-editor-states-row-1-to', 'down')

    expect(emitted(w).at(-1)).toEqual({
      ...base,
      map: { state: '/raw/body/status', states: { '1': 'resolved', '0': 'down' } },
    })

    await w.get('[data-testid="sources.relay-editor-states-row-0-remove"]').trigger('click')

    expect(w.find('[data-testid="sources.relay-editor-states-row-1"]').exists()).toBe(false)
    expect(emitted(w).at(-1)).toEqual({ ...base, map: { state: '/raw/body/status', states: { '0': 'down' } } })
  })

  it('renders the recent deliveries', async () => {
    const client = fakeClient({ recentDeliveries: vi.fn(() => Promise.resolve([delivery('d1'), delivery('d2')])) })
    const w = mountEditor(base, client)
    await flushPromises()

    expect(client.recentDeliveries).toHaveBeenCalledWith(ACCOUNT, 'src-1', 5)
    const recent = w.get('[data-testid="sources.relay-editor-recent"]')
    expect(recent.findAll('pre')).toHaveLength(2)
    expect(w.get('[data-testid="sources.relay-editor-recent-d1"]').text()).toContain('generic')
    expect(w.get('[data-testid="sources.relay-editor-recent-d1"] pre').text()).toContain('"title": "Build failed"')
    expect(w.find('[data-testid="sources.relay-editor-no-recent"]').exists()).toBe(false)
  })

  it('shows the empty state with no deliveries and reloads on a source change', async () => {
    const client = fakeClient()
    const w = mountEditor(base, client)
    await flushPromises()

    expect(w.find('[data-testid="sources.relay-editor-no-recent"]').exists()).toBe(true)
    await w.setProps({ config: { ...base, source: 'src-2' } })
    await flushPromises()
    expect(client.recentDeliveries).toHaveBeenLastCalledWith(ACCOUNT, 'src-2', 5)
  })
})

describe('sources.relay validate', () => {
  it.each<[string, Config, string[]]>([
    [
      'fresh defaults',
      { ...defaults },
      ['a source needs a connected Hive Relay account', 'pick or create a relay source'],
    ],
    ['a valid config', base, []],
    [
      'a credential for another provider',
      { ...base, credential: 'gitea/git.example.com-me' },
      ['credential must look like "relay/<host>-<email>"'],
    ],
    [
      'an unavailable dialect',
      { ...base, dialect: 'ntfy' },
      ['format "ntfy" arrives in a later release; use auto or generic'],
    ],
    [
      'a pointer without a leading slash',
      { ...base, map: { key: 'raw/body/id', title: '' } },
      ['map.key must be a JSON Pointer starting with "/"'],
    ],
  ])('%s', (_name, config, errors) => {
    expect(validate(config)).toEqual(errors)
  })
})
