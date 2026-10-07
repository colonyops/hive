import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ActionRunsView from '../ActionRunsView.vue'

const mocks = vi.hoisted(() => ({
  List: vi.fn(),
  Log: vi.fn(),
  Cancel: vi.fn(),
  ActionRun: vi.fn(),
  SetText: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/actionrunservice', () => ({
  List: mocks.List,
  Log: mocks.Log,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/jobservice', () => ({
  Cancel: mocks.Cancel,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice', () => ({
  ActionRun: mocks.ActionRun,
}))
vi.mock('../../composables/useWailsEvent', () => ({ useWailsEvent: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: mocks.SetText } }))

const runs = [
  {
    id: 12,
    actionId: 'deploy',
    label: 'Deploy',
    type: 'shell',
    status: 'failed',
    attempts: 2,
    lane: 'automatic',
    rerun: false,
    key: 'item-1',
    createdAt: 1_000,
    startedAt: 2_000,
    finishedAt: 3_000,
    error: 'exit status 3',
    itemId: 7,
    itemTitle: 'Ship it',
  },
  {
    id: 11,
    actionId: 'lint',
    label: 'Lint',
    status: 'done',
    attempts: 1,
    lane: 'manual',
    rerun: false,
    key: 'item-2',
    createdAt: 500,
    startedAt: 600,
    finishedAt: 700,
  },
]

describe('ActionRunsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.List.mockResolvedValue(runs)
    mocks.Log.mockImplementation((id: number) =>
      Promise.resolve(
        id === 12
          ? {
              lines: [
                { id: 1, attempt: 1, stream: 'system', text: 'Started Deploy', at: 2_000 },
                { id: 2, attempt: 2, stream: 'stdout', text: 'deploying', at: 2_100 },
                { id: 3, attempt: 2, stream: 'stderr', text: 'boom', at: 2_200 },
              ],
              nextAfterId: 3,
              more: false,
            }
          : { lines: [], nextAfterId: 0, more: false },
      ),
    )
    mocks.ActionRun.mockResolvedValue({ commandId: 11, status: 'done', stdout: 'legacy output' })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('opens on the newest run and shows its log by attempt', async () => {
    const wrapper = mount(ActionRunsView)
    await flushPromises()

    expect(wrapper.findAll('[data-testid="action-run-row"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="action-run-title"]').text()).toBe('Deploy')
    expect(wrapper.get('[data-testid="action-run-status"]').text()).toBe('Failed')
    expect(wrapper.get('[data-testid="action-run-error"]').text()).toBe('exit status 3')
    expect(wrapper.findAll('[data-testid="action-run-attempt"]').map((h) => h.text())).toEqual([
      'Attempt 1',
      'Attempt 2',
    ])
    const streams = wrapper.findAll('[data-testid="action-run-line"]').map((line) => line.attributes('data-stream'))
    expect(streams).toEqual(['system', 'stdout', 'stderr'])
  })

  it('selects the run it was opened on and falls back to recorded output', async () => {
    const wrapper = mount(ActionRunsView, { props: { initialRunId: 11 } })
    await flushPromises()

    expect(wrapper.get('[data-testid="action-run-title"]').text()).toBe('Lint')
    expect(mocks.ActionRun).toHaveBeenCalledWith(11)
    expect(wrapper.get('[data-testid="action-run-log"]').text()).toContain('legacy output')
  })

  it('moves between runs from the keyboard and opens the run item', async () => {
    const wrapper = mount(ActionRunsView)
    await flushPromises()

    await wrapper.get('[data-testid="action-runs-list"]').trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    expect(wrapper.get('[data-testid="action-run-title"]').text()).toBe('Lint')
    expect(wrapper.find('[data-testid="action-run-open-item"]').exists()).toBe(false)

    await wrapper.get('[data-testid="action-runs-list"]').trigger('keydown', { key: 'k' })
    await flushPromises()
    await wrapper.get('[data-testid="action-run-open-item"]').trigger('click')
    expect(wrapper.emitted('open-item')).toEqual([[12, 'deploy']])
  })

  it('filters runs by search', async () => {
    const wrapper = mount(ActionRunsView)
    await flushPromises()
    await wrapper.get('input[data-testid="action-runs-search"]').setValue('lint')
    expect(wrapper.findAll('[data-testid="action-run-row"]').map((row) => row.attributes('data-run-id'))).toEqual([
      '11',
    ])
  })
})
