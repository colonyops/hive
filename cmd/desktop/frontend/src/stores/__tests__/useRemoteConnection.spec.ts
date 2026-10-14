import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { resetStores } from '../defineStore'
import { useRemoteConnection } from '../useRemoteConnection'

const list = vi.hoisted(() => vi.fn())
vi.mock('../../lib/remoteConnection', async (original) => ({
  ...(await original<typeof import('../../lib/remoteConnection')>()),
  listRemoteSessions: list,
}))
beforeEach(() => {
  list.mockReset()
  localStorage.clear()
})
const profile = (name = 'Remote', color = '#a78bfa') => ({ id: '', name, address: 'http://127.0.0.1:19001', color })

describe('remote connections', () => {
  it('saves independent names and colors without persisting credentials', async () => {
    const remote = useRemoteConnection()
    const a = remote.save(profile('First', '#ff0000'))
    remote.save(profile('Second', '#00ff00'))
    list.mockResolvedValue([])
    await remote.connect(a, 'private-token')
    await nextTick()
    expect(remote.color.value).toBe('#ff0000')
    expect(remote.profiles.value.map((row) => row.color)).toEqual(['#ff0000', '#00ff00'])
    expect(localStorage.getItem('hive.remote.profiles')).not.toContain('private-token')
    remote.save({ ...remote.profiles.value[0], color: '#0000ff' })
    expect(remote.color.value).toBe('#0000ff')
    expect(remote.endpoint.value?.token).toBe('private-token')
    await nextTick()
    resetStores()
    const reopened = useRemoteConnection()
    expect(reopened.profiles.value).toHaveLength(2)
    expect(reopened.profiles.value.find((row) => row.id === a)?.color).toBe('#0000ff')
    expect(reopened.endpoint.value).toBeNull()
  })
  it('cannot restore a pending connection after its profile is removed', async () => {
    const remote = useRemoteConnection()
    const id = remote.save(profile())
    let resolve!: (rows: unknown[]) => void
    list.mockReturnValue(
      new Promise((r) => {
        resolve = r
      }),
    )
    const pending = remote.connect(id, 'secret')
    remote.remove(id)
    resolve([])
    expect(await pending).toBe(false)
    expect(remote.endpoint.value).toBeNull()
  })
  it('disconnects before changing the active endpoint', async () => {
    const remote = useRemoteConnection()
    const id = remote.save(profile())
    list.mockResolvedValue([])
    await remote.connect(id, 'secret')
    remote.save({ ...profile(), id, address: 'http://127.0.0.1:19002' })
    expect(remote.endpoint.value).toBeNull()
  })
})
