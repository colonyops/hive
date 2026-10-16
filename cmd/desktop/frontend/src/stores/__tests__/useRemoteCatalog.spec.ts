import { beforeEach, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { useRemoteCatalog } from '../useRemoteCatalog'
import { useRemoteConnection } from '../useRemoteConnection'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  workspaces: vi.fn(),
  allSessions: vi.fn(),
  openWorkspace: vi.fn(),
  startSession: vi.fn(),
  resumeSession: vi.fn(),
  inboxes: vi.fn(),
}))
vi.mock('../../lib/remoteConnection', async (original) => ({
  ...(await original<typeof import('../../lib/remoteConnection')>()),
  listRemoteSessions: mocks.list,
}))
vi.mock('../../lib/agentWorkspacesClient', () => ({ createAgentWorkspacesClient: () => mocks }))
vi.mock('../../lib/remoteInboxClient', () => ({ createRemoteInboxClient: () => mocks }))
beforeEach(() => {
  vi.resetAllMocks()
  localStorage.clear()
  mocks.list.mockResolvedValue([])
  mocks.workspaces.mockResolvedValue({ available: true, workspaces: [{ dir: 'shared-name', name: 'Remote shell' }] })
  mocks.allSessions.mockResolvedValue([])
  mocks.inboxes.mockResolvedValue([{ id: 'shared-name', name: 'Remote inbox', feeds: [] }])
})
async function connected() {
  const catalog = useRemoteCatalog()
  const remote = useRemoteConnection()
  const id = remote.save({
    id: '',
    name: 'Purple',
    host: 'hive-server',
    connectionFile: '~/connection.json',
    color: '#a855f7',
  })
  await remote.connect(id)
  await flushPromises()
  return { catalog, remote }
}
it('starts and resumes chats using the selected remote workspace', async () => {
  const { catalog } = await connected()
  catalog.selectWorkspace('shared-name')
  mocks.startSession.mockResolvedValue({ id: 'remote-chat', workspace: 'shared-name', terminalId: 'agentws-remote' })
  await catalog.openChat('Test chat')
  expect(mocks.openWorkspace).toHaveBeenCalledWith('shared-name')
  expect(mocks.startSession).toHaveBeenCalledWith({ workspace: 'shared-name', name: 'Test chat' })
  expect(catalog.chat.value?.id).toBe('remote-chat')
  mocks.resumeSession.mockResolvedValue({ id: 'remote-chat', terminalId: 'agentws-remote' })
  await catalog.openChat('', 'remote-chat')
  expect(mocks.resumeSession).toHaveBeenCalledWith({ id: 'remote-chat' })
})
it('discards catalogs and a pending chat result when disconnected', async () => {
  const { catalog, remote } = await connected()
  expect(catalog.inboxes.value).toHaveLength(1)
  catalog.selectInbox('shared-name')
  catalog.selectWorkspace('shared-name')
  let resolve!: (value: unknown) => void
  mocks.startSession.mockReturnValue(
    new Promise((r) => {
      resolve = r
    }),
  )
  const pending = catalog.openChat('Pending')
  await flushPromises()
  remote.disconnect()
  resolve({ id: 'old-remote-chat' })
  await pending
  expect(catalog.inboxes.value).toEqual([])
  expect(catalog.workspaces.value).toEqual([])
  expect(catalog.chat.value).toBeNull()
  expect(catalog.inboxId.value).toBe('')
  expect(catalog.workspace.value).toBe('')
})

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/remoteservice', () => ({
  Connect: vi.fn(() => Promise.resolve('remote-one')),
  Disconnect: vi.fn(async () => {}),
}))

it('preserves selected inbox, code and chat across a server reconnect', async () => {
  const { catalog, remote } = await connected()
  catalog.selectInbox('shared-name')
  catalog.selectCode('code-one')
  catalog.selectWorkspace('shared-name')
  mocks.startSession.mockResolvedValue({ id: 'remote-chat', terminalId: 'agentws-remote' })
  await catalog.openChat('Chat')
  await remote.reconnect()
  await flushPromises()
  expect(catalog.inboxId.value).toBe('shared-name')
  expect(catalog.codeId.value).toBe('code-one')
  expect(catalog.workspace.value).toBe('shared-name')
  expect(catalog.chat.value?.id).toBe('remote-chat')
})
