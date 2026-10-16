import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useRemoteCode } from '../useRemoteCode'
import type { TerminalEndpoint } from '../../lib/terminalClient'

const mocks = vi.hoisted(() => ({
  options: vi.fn(),
  create: vi.fn(),
  job: vi.fn(),
  statuses: vi.fn(),
  windows: vi.fn(),
  reload: vi.fn(),
}))
const remote = {
  endpoint: ref<TerminalEndpoint | null>(null),
  activeProfile: ref<{ id: string } | null>(null),
  sessions: ref([{ id: 'one', slug: 'same-slug', name: 'review', state: 'active', repo: 'acme/site' }]),
  reload: mocks.reload,
}
vi.mock('../useRemoteConnection', () => ({ useRemoteConnection: () => remote }))
vi.mock('../../lib/remoteCodeClient', () => ({ createRemoteCodeClient: () => mocks }))
vi.mock('../../lib/terminalClient', () => ({ createTerminalClient: () => ({ listWindows: mocks.windows }) }))
beforeEach(() => {
  vi.resetAllMocks()
  remote.endpoint.value = { httpBaseURL: 'hive-remote://one', token: '', wsURL: '' }
  remote.activeProfile.value = { id: 'one' }
  mocks.options.mockResolvedValue({ repositories: [], agents: ['shell'], defaultAgent: 'shell' })
  mocks.statuses.mockResolvedValue({ items: [] })
  mocks.windows.mockResolvedValue({ 'same-slug': [{ windowId: '@1', name: 'shell' }] })
  mocks.create.mockResolvedValue({ jobId: 12 })
  mocks.job.mockResolvedValue({ status: 'done' })
})
const input = { repository: 'acme/site', name: 'review', agent: 'shell', prompt: '' }
describe('remote Code', () => {
  it('uses the remote windows and agents and observes creation completion', async () => {
    const code = useRemoteCode()
    await flushPromises()
    expect(code.options.value?.defaultAgent).toBe('shell')
    await code.create('one', input)
    expect(mocks.create).toHaveBeenCalledWith(input)
    expect(mocks.job).toHaveBeenCalledWith(12)
    expect(mocks.reload).toHaveBeenCalledOnce()
    expect(code.windows.value['same-slug'][0].name).toBe('shell')
  })
  it('rejects a disconnected or different installation before starting anything', async () => {
    const code = useRemoteCode()
    await expect(code.create('other', input)).rejects.toThrow('Reconnect')
    expect(mocks.create).not.toHaveBeenCalled()
  })
  it('reports job failure instead of claiming the session was created', async () => {
    mocks.job.mockResolvedValue({ status: 'failed', error: 'repository inaccessible' })
    await expect(useRemoteCode().create('one', input)).rejects.toThrow('repository inaccessible')
    expect(mocks.reload).not.toHaveBeenCalled()
  })
  it('does not refresh another installation when an old creation finishes', async () => {
    let finish!: (value: { status: string }) => void
    mocks.job.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve
      }),
    )
    const code = useRemoteCode()
    const pending = code.create('one', input)
    await flushPromises()
    remote.endpoint.value = { httpBaseURL: 'hive-remote://two', token: '', wsURL: '' }
    remote.activeProfile.value = { id: 'two' }
    await flushPromises()
    finish({ status: 'done' })
    await pending
    expect(mocks.reload).not.toHaveBeenCalled()
  })
})

it('preserves the selected window only when reconnecting the same profile', async () => {
  const code = useRemoteCode()
  code.selectWindow('@2')
  remote.endpoint.value = { httpBaseURL: 'hive-remote://reconnected', token: '', wsURL: '' }
  await flushPromises()
  expect(code.selectedWindow.value).toBe('@2')
  remote.activeProfile.value = { id: 'two' }
  remote.endpoint.value = { httpBaseURL: 'hive-remote://other', token: '', wsURL: '' }
  await flushPromises()
  expect(code.selectedWindow.value).toBe('')
})
