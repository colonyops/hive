import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useRemoteConnection } from '../../stores/useRemoteConnection'
import { useRemoteCatalog } from '../../stores/useRemoteCatalog'
import RemoteCodeTree from '../terminal/RemoteCodeTree.vue'
import RemoteCode from '../RemoteCode.vue'

vi.mock('vue-router', () => ({ useRoute: () => ({ name: 'terminal' }), useRouter: () => ({ push: vi.fn() }) }))

const mocks = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('../../lib/remoteConnection', async (original) => ({
  ...(await original<typeof import('../../lib/remoteConnection')>()),
  listRemoteSessions: mocks.list,
}))
vi.mock('../RemoteTerminal.vue', () => ({
  default: {
    props: ['slug', 'endpoint'],
    setup: (props: { slug: string }) => ({ initialSlug: props.slug }),
    template: '<div data-testid="attached">{{ initialSlug }}</div>',
  },
}))
vi.mock('../../composables/useTooltip', () => ({ useTooltip: () => ({ triggers: {}, show: { value: false } }) }))

beforeEach(() => {
  mocks.list.mockReset()
})

async function connect() {
  const remote = useRemoteConnection()
  const id = remote.save({
    id: '',
    name: 'Test remote',
    host: 'hive-server',
    connectionFile: '~/connection.json',
    color: '#a78bfa',
  })
  await remote.connect(id)
  await flushPromises()
}

describe('remote Code isolation', () => {
  it('keeps last-known rows during an outage and detaches when hidden', async () => {
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Remote', slug: 'same-slug', state: 'active', repo: '' }])
    const wrapper = mount(RemoteCode, { props: { active: true } })
    await connect()
    const navigation = mount(RemoteCodeTree)
    await navigation.get('[data-testid="remote-code-item-one"]').trigger('click')
    expect(wrapper.get('[data-testid="attached"]').text()).toBe('same-slug')
    mocks.list.mockRejectedValue(new Error('Tunnel disconnected'))
    await wrapper.get('[data-testid="remote-refresh"]').trigger('click')
    await flushPromises()
    expect(navigation.find('[data-testid="remote-code-item-one"]').exists()).toBe(true)
    navigation.unmount()
    expect(wrapper.get('[data-testid="remote-error"]').text()).toContain('Tunnel disconnected')
    await wrapper.setProps({ active: false })
    expect(wrapper.find('[data-testid="attached"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('reattaches when a session is renamed with the same ID', async () => {
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Before', slug: 'before', state: 'active', repo: '' }])
    const wrapper = mount(RemoteCode, { props: { active: true } })
    await connect()
    useRemoteCatalog().selectCode('one')
    await flushPromises()
    expect(wrapper.get('[data-testid="attached"]').text()).toBe('before')
    mocks.list.mockResolvedValue([{ id: 'one', name: 'After', slug: 'after', state: 'active', repo: '' }])
    await wrapper.get('[data-testid="remote-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="attached"]').text()).toBe('after')
    wrapper.unmount()
  })

  it('does not restore a connection when a refresh completes after disconnect', async () => {
    mocks.list.mockResolvedValue([])
    const wrapper = mount(RemoteCode, { props: { active: true } })
    await connect()
    let resolve!: (rows: unknown[]) => void
    mocks.list.mockReturnValue(
      new Promise((r) => {
        resolve = r
      }),
    )
    await wrapper.get('[data-testid="remote-refresh"]').trigger('click')
    useRemoteConnection().disconnect()
    resolve([{ id: 'one', name: 'Remote', slug: 'remote', state: 'active', repo: '' }])
    await flushPromises()
    expect(wrapper.find('[data-testid="remote-not-connected"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="remote-sessions"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/remoteservice', () => ({
  Connect: vi.fn(() => Promise.resolve('remote-one')),
  Disconnect: vi.fn(async () => {}),
}))
