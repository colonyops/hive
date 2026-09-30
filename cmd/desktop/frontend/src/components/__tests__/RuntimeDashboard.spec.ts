import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RuntimeDashboard from '../RuntimeDashboard.vue'

const mocks = vi.hoisted(() => ({
  Stats: vi.fn(),
  startFrameStats: vi.fn(),
  stopFrameStats: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/observabilityservice', () => ({
  Stats: mocks.Stats,
}))

vi.mock('../../composables/useFrameStats', async () => {
  const { shallowRef } = await vi.importActual<typeof import('vue')>('vue')
  const stats = shallowRef({
    fps: 118.4,
    frameMs: 8.4,
    worstFrameMs: 184,
    dropped: 3,
    windowMs: 10_000,
    lagMs: 2,
    worstLagMs: 96,
    buckets: [8, 9, 184, 8],
  })
  return {
    startFrameStats: mocks.startFrameStats,
    stopFrameStats: mocks.stopFrameStats,
    useFrameStats: () => ({ stats, running: shallowRef(true) }),
  }
})

const MB = 1024 * 1024

function runtimeSample(overrides: Record<string, unknown> = {}) {
  return {
    sampledAtUnixMs: 1_770_000_000_000,
    uptimeMs: 2 * 3_600_000 + 14 * 60_000,
    process: { pid: 100, name: 'hive-desktop', rssBytes: 214 * MB, cpuPercent: 3.4, threads: 42 },
    children: [{ pid: 101, name: 'agent', rssBytes: 180 * MB, cpuPercent: 1.2, threads: 12 }],
    childrenTruncated: false,
    totalRssBytes: 394 * MB,
    totalCpuPercent: 4.6,
    go: {
      goroutines: 87,
      gomaxprocs: 10,
      numCpu: 12,
      heapAllocBytes: 41 * MB,
      heapSysBytes: 68 * MB,
      heapObjects: 120_000,
      stackSysBytes: 2 * MB,
      totalSysBytes: 92 * MB,
      nextGcBytes: 82 * MB,
      gcCount: 142,
      lastGcUnixMs: 1_770_000_000_000,
      lastPauseMs: 0.4,
      totalPauseMs: 61,
    },
    ...overrides,
  }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  mocks.Stats.mockResolvedValue(runtimeSample())
})

afterEach(() => {
  vi.useRealTimers()
})

describe('RuntimeDashboard', () => {
  it('reports the process tree, Go runtime, and UI frames', async () => {
    const wrapper = mount(RuntimeDashboard)
    await flushPromises()

    expect(mocks.startFrameStats).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="observability-runtime-details"]').attributes('open')).toBeUndefined()
    expect(wrapper.find('[data-testid="observability-runtime-poll"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="observability-runtime-refresh"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="observability-runtime-memory"]').text()).toContain('394 MB')
    expect(wrapper.get('[data-testid="observability-runtime-cpu"]').text()).toContain('4.6%')

    const vitals = wrapper.get('[data-testid="observability-runtime-vitals"]').text()
    expect(vitals).toContain('87')
    expect(vitals).toContain('2h 14m')
    expect(vitals).toContain('10 of 12')
    expect(vitals).toContain('2 / 96ms')

    const processes = wrapper.get('[data-testid="observability-runtime-processes"]').text()
    expect(processes).toContain('hive-desktop')
    expect(processes).toContain('agent')

    const frames = wrapper.get('[data-testid="observability-runtime-frames"]').text()
    expect(frames).toContain('118')
    expect(frames).toContain('worst 184ms')
    expect(frames).toContain('3 dropped in 10s')

    wrapper.unmount()
    expect(mocks.stopFrameStats).toHaveBeenCalledOnce()
  })

  it('states the heap against the next collection target', async () => {
    const wrapper = mount(RuntimeDashboard)
    await flushPromises()

    const go = wrapper.get('[data-testid="observability-runtime-go"]').text()
    expect(go).toContain('41.0 MB')
    expect(go).toContain('of 82.0 MB')
    expect(wrapper.get('[data-testid="observability-runtime-heap-pressure"]').text()).toBe('50% of next GC target')

    wrapper.unmount()
  })

  it('says the webview is not counted when no child process is present', async () => {
    mocks.Stats.mockResolvedValue(runtimeSample({ children: [], totalRssBytes: 214 * MB, totalCpuPercent: 3.4 }))
    const wrapper = mount(RuntimeDashboard)
    await flushPromises()

    expect(wrapper.get('[data-testid="observability-runtime-processes"]').text()).toContain("webview's rendering helpers are not")

    wrapper.unmount()
  })

  it('surfaces a failed sample', async () => {
    mocks.Stats.mockRejectedValue(new Error('process is gone'))
    const wrapper = mount(RuntimeDashboard)
    await flushPromises()

    expect(wrapper.get('[data-testid="observability-runtime-error"]').text()).toContain('process is gone')

    wrapper.unmount()
  })
})
