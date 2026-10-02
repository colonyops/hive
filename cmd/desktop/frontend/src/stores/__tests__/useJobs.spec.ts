import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../defineStore'
import { useJobs } from '../useJobs'

const mocks = vi.hoisted(() => ({
  ListActive: vi.fn(),
  On: vi.fn(),
  unsubscribe: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/jobservice', () => ({
  ListActive: mocks.ListActive,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function job(status: string, id = 1) {
  return {
    id,
    createdAt: 1,
    updatedAt: 1,
    status,
    label: 'Review',
    step: status,
    actionId: 'review',
    target: 'item-1',
    commandId: 12,
  }
}

function fireJobsUpdated(): void {
  const handler = mocks.On.mock.calls[0][1] as () => void
  handler()
}

describe('useJobs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(mocks.unsubscribe)
    mocks.ListActive.mockResolvedValue([])
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('subscribes once and reloads on jobs:updated', async () => {
    const jobs = useJobs()
    await flushPromises()
    expect(useJobs()).toBe(jobs)
    expect(mocks.On).toHaveBeenCalledTimes(1)
    expect(mocks.On.mock.calls[0][0]).toBe('jobs:updated')

    mocks.ListActive.mockResolvedValue([job('running')])
    fireJobsUpdated()
    await vi.waitFor(() => expect(jobs.hasActive.value).toBe(true))
  })

  it('reads once more after a request that an update overlapped', async () => {
    let resolveFirst!: (rows: ReturnType<typeof job>[]) => void
    mocks.ListActive.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve
        }),
    ).mockResolvedValueOnce([job('running', 2)])

    const jobs = useJobs()
    fireJobsUpdated()
    expect(mocks.ListActive).toHaveBeenCalledTimes(1)

    resolveFirst([])
    await flushPromises()

    expect(mocks.ListActive).toHaveBeenCalledTimes(2)
    expect(jobs.activeJobs.value.map((row) => row.id)).toEqual([2])
  })

  it('retries a failed trailing read while terminal rows remain', async () => {
    vi.useFakeTimers()
    mocks.ListActive.mockResolvedValueOnce([job('done')])
      .mockRejectedValueOnce(new Error('temporary failure'))
      .mockResolvedValueOnce([])

    const jobs = useJobs()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()
    expect(jobs.hasActive.value).toBe(true)
    expect(console.warn).toHaveBeenCalledWith('Unable to load active jobs:', 'temporary failure')
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()

    expect(mocks.ListActive).toHaveBeenCalledTimes(3)
    expect(jobs.hasActive.value).toBe(false)
  })

  it('keeps polling terminal rows until the backend drops them', async () => {
    vi.useFakeTimers()
    mocks.ListActive.mockResolvedValueOnce([job('done')]).mockResolvedValueOnce([])

    const jobs = useJobs()
    await flushPromises()
    expect(jobs.hasActive.value).toBe(true)
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()

    expect(mocks.ListActive).toHaveBeenCalledTimes(2)
    expect(jobs.hasActive.value).toBe(false)
  })

  it('unsubscribes and drops the trailing read when the store resets', async () => {
    vi.useFakeTimers()
    mocks.ListActive.mockResolvedValueOnce([job('done')])

    useJobs()
    await flushPromises()
    resetStores()
    await vi.advanceTimersByTimeAsync(500)

    expect(mocks.unsubscribe).toHaveBeenCalledOnce()
    expect(mocks.ListActive).toHaveBeenCalledTimes(1)
  })
})
