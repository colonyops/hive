import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  ListActive: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/jobservice', () => ({
  ListActive: mocks.ListActive,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function job(status: string, id = 1) {
  return {
    id, createdAt: 1, updatedAt: 1, status, label: 'Review', step: status,
    actionId: 'review', target: 'item-1', commandId: 12,
  }
}

async function loadComposable() {
  const { useJobs } = await import('../useJobs')
  return useJobs()
}

describe('useJobs', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.ListActive.mockResolvedValue([])
  })

  afterEach(() => vi.useRealTimers())

  it('subscribes once and reloads on jobs:updated', async () => {
    const jobs = await loadComposable()
    await jobs.load()
    const again = await loadComposable()
    expect(again.activeJobs).toBe(jobs.activeJobs)
    expect(mocks.On).toHaveBeenCalledTimes(1)
    expect(mocks.On.mock.calls[0][0]).toBe('jobs:updated')

    mocks.ListActive.mockResolvedValue([job('running')])
    mocks.On.mock.calls[0][1]()
    await vi.waitFor(() => expect(jobs.hasActive.value).toBe(true))
  })

  it('drops a stale earlier read', async () => {
    let resolveFirst!: (rows: ReturnType<typeof job>[]) => void
    let resolveSecond!: (rows: ReturnType<typeof job>[]) => void
    mocks.ListActive
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveSecond = resolve }))

    const jobs = await loadComposable()
    const latest = jobs.load()
    resolveSecond([job('running', 2)])
    await latest
    resolveFirst([])
    await Promise.resolve()

    expect(jobs.activeJobs.value.map((row) => row.id)).toEqual([2])
  })

  it('retries a failed trailing read while terminal rows remain', async () => {
    vi.useFakeTimers()
    mocks.ListActive
      .mockResolvedValueOnce([job('done')])
      .mockRejectedValueOnce(new Error('temporary failure'))
      .mockResolvedValueOnce([])

    const jobs = await loadComposable()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()
    expect(jobs.hasActive.value).toBe(true)
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()

    expect(mocks.ListActive).toHaveBeenCalledTimes(3)
    expect(jobs.hasActive.value).toBe(false)
  })

  it('keeps polling terminal rows until the backend drops them', async () => {
    vi.useFakeTimers()
    mocks.ListActive
      .mockResolvedValueOnce([job('done')])
      .mockResolvedValueOnce([])

    const jobs = await loadComposable()
    await flushPromises()
    expect(jobs.hasActive.value).toBe(true)
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()

    expect(mocks.ListActive).toHaveBeenCalledTimes(2)
    expect(jobs.hasActive.value).toBe(false)
  })
})
