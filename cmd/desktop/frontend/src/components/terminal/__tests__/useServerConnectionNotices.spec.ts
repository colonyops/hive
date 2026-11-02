import { describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import type { NotifyEvent } from '../../../composables/useNotify'
import { useServerConnectionNotices } from '../useServerConnectionNotices'

function setup() {
  const servers = ref<{ name: string; state: string; error: string }[]>([])
  const listError = ref<string | null>(null)
  const notify = vi.fn<(event: NotifyEvent) => Promise<void>>().mockResolvedValue(undefined)
  useServerConnectionNotices(
    () => servers.value,
    () => listError.value,
    notify,
  )
  return { servers, listError, notify }
}

const down = { name: 'box', state: 'offline', error: 'ssh: connect refused' }

describe('useServerConnectionNotices', () => {
  it('notifies once while a server stays offline', async () => {
    const { servers, notify } = setup()
    servers.value = [down]
    await nextTick()
    servers.value = [{ ...down }]
    await nextTick()
    expect(notify).toHaveBeenCalledTimes(1)
    expect(notify).toHaveBeenCalledWith({ title: 'box is offline', body: 'ssh: connect refused', severity: 'error' })
  })

  it('notifies again when the state changes or the server fails after it recovers', async () => {
    const { servers, notify } = setup()
    servers.value = [down]
    await nextTick()
    servers.value = [{ ...down, state: 'incompatible' }]
    await nextTick()
    servers.value = []
    await nextTick()
    servers.value = [down]
    await nextTick()
    expect(notify.mock.calls.map(([event]) => event.title)).toEqual([
      'box is offline',
      'box is incompatible',
      'box is offline',
    ])
  })

  it('notifies when the server list fails outright', async () => {
    const { listError, notify } = setup()
    listError.value = 'no fleet'
    await nextTick()
    expect(notify).toHaveBeenCalledWith({ title: 'Could not reach your servers', body: 'no fleet', severity: 'error' })
  })
})
