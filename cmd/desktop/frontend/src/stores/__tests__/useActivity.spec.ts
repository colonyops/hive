import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useActivity } from '../useActivity'

const mocks = vi.hoisted(() => ({
  List: vi.fn(),
  Record: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/activityservice', () => ({
  List: mocks.List,
  Record: mocks.Record,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function event(id: number) {
  return { id, category: 'system', severity: 'info', title: `event ${id}`, body: '', source: '', metadata: null }
}

function fireAppended(): void {
  const handler = mocks.On.mock.calls.find(([name]) => name === 'activity:appended')?.[1] as () => void
  handler()
}

describe('useActivity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.List.mockResolvedValue([])
    mocks.Record.mockResolvedValue(undefined)
  })

  afterEach(() => vi.restoreAllMocks())

  it('loads the latest page on first use and treats it as already seen', async () => {
    mocks.List.mockResolvedValue([event(3), event(2), event(1)])

    const activity = useActivity()
    await flushPromises()

    expect(mocks.List).toHaveBeenCalledWith(0, 200)
    expect(activity.events.value.map((row) => row.id)).toEqual([3, 2, 1])
    expect(activity.unseenCount.value).toBe(0)
    expect(mocks.On.mock.calls[0][0]).toBe('activity:appended')
  })

  it('counts events that arrive later as unseen until markSeen', async () => {
    mocks.List.mockResolvedValueOnce([event(2), event(1)])
    const activity = useActivity()
    await flushPromises()

    mocks.List.mockResolvedValue([event(4), event(3), event(2), event(1)])
    fireAppended()
    await vi.waitFor(() => expect(activity.unseenCount.value).toBe(2))

    activity.markSeen()
    expect(activity.unseenCount.value).toBe(0)
  })

  it('seeds the seen marker on the first load that succeeds', async () => {
    mocks.List.mockRejectedValueOnce(new Error('boom'))
    const activity = useActivity()
    await flushPromises()
    expect(activity.error.value).toBe('boom')
    expect(activity.events.value).toEqual([])

    mocks.List.mockResolvedValue([event(5)])
    await activity.reload()

    expect(activity.error.value).toBeNull()
    expect(activity.unseenCount.value).toBe(0)
  })

  it('fills record defaults and reports failure instead of throwing', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const activity = useActivity()
    await flushPromises()

    expect(await activity.record({ title: 'Hello' })).toBe(true)
    expect(mocks.Record).toHaveBeenCalledWith({
      category: '',
      severity: '',
      title: 'Hello',
      body: '',
      source: '',
      metadata: null,
    })

    mocks.Record.mockRejectedValueOnce(new Error('down'))
    expect(await activity.record({ title: 'Again' })).toBe(false)
    expect(console.error).toHaveBeenCalledOnce()
  })
})
