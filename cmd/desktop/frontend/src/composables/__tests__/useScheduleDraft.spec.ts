import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { useScheduleDraft } from '../useScheduleDraft'
import type { AgentSchedule } from '../../lib/agentWorkspacesClient'

const api = vi.hoisted(() => ({
  runs: vi.fn(),
  runNow: vi.fn(),
  preview: vi.fn(),
}))
vi.mock('../useAgentSchedules', () => ({ useAgentSchedules: () => api }))

function schedule(overrides: Partial<AgentSchedule> = {}): AgentSchedule {
  return {
    id: 'weekly-summary',
    name: 'Weekly summary',
    cron: '0 9 * * 5',
    prompt: 'Summarize the week.',
    disabled: false,
    onMissed: 'run',
    nextRunAt: null,
    lastRun: null,
    ...overrides,
  }
}

function setup(schedules: AgentSchedule[] = [schedule()], workspace = 'demo') {
  const scope = effectScope()
  const drafts = scope.run(() =>
    useScheduleDraft({ schedules: () => schedules, workspace: () => workspace, scroller: () => null }),
  )!
  return { scope, ...drafts }
}

beforeEach(() => {
  api.runs.mockResolvedValue([])
  api.runNow.mockResolvedValue({ id: 1, startedAt: 1, reason: 'manual', status: 'launched', missed: 0, error: '' })
  api.preview.mockResolvedValue({ next: [1, 2, 3, 4], cronError: '', promptError: '' })
})

afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('useScheduleDraft', () => {
  it('seeds a card per schedule and compiles them back to edits', () => {
    const { cards, edits } = setup()
    expect(cards.value).toHaveLength(1)
    expect(edits()).toEqual([
      {
        id: 'weekly-summary',
        name: 'Weekly summary',
        cron: '0 9 * * 5',
        prompt: 'Summarize the week.',
        disabled: false,
        onMissed: 'run',
      },
    ])
  })

  it('adds a new schedule whose id follows its name', () => {
    const { cards, draft, open, keep } = setup([])
    open(null)
    draft.value!.name = 'Daily Digest!'
    draft.value!.prompt = 'Digest.'
    keep()
    expect(draft.value).toBeNull()
    expect(cards.value.map((card) => [card.id, card.saved])).toEqual([['daily-digest', false]])
  })

  it('reports what blocks keep only after it is tried, in order', () => {
    const { draft, issue, open, keep } = setup()
    open(null)
    expect(issue.value).toBe('')
    keep()
    expect(issue.value).toBe('A name is required.')
    draft.value!.name = 'Weekly summary'
    expect(issue.value).toBe('Another schedule already uses the id "weekly-summary".')
    draft.value!.name = 'Other'
    expect(issue.value).toBe('A prompt is required.')
  })

  it('marks a saved card edited only when its timetable changes', () => {
    const { cards, draft, open, keep } = setup()
    open(cards.value[0])
    draft.value!.prompt = 'New prompt.'
    keep()
    expect(cards.value[0].edited).toBe(false)
    open(cards.value[0])
    draft.value!.shape = { kind: 'daily', hour: 8, minute: 0 }
    keep()
    expect(cards.value[0].edited).toBe(true)
  })

  it('removes the card the draft was opened on', () => {
    const { cards, open, remove } = setup()
    open(cards.value[0])
    remove()
    expect(cards.value).toEqual([])
  })

  it('debounces the preview and keeps the first occurrences', async () => {
    vi.useFakeTimers()
    const { cards, draft, open } = setup()
    open(cards.value[0])
    await nextTick()
    draft.value!.prompt = 'Changed.'
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)
    expect(api.preview).toHaveBeenCalledTimes(1)
    expect(api.preview).toHaveBeenCalledWith({ workspace: 'demo', cron: '0 9 * * 5', prompt: 'Changed.' })
    expect(draft.value!.next).toEqual([1, 2, 3])
    expect(draft.value!.previewed).toBe(true)
  })

  it('asks nothing for a weekly shape with no days, or before the workspace exists', async () => {
    vi.useFakeTimers()
    const saved = setup()
    saved.open(saved.cards.value[0])
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)
    api.preview.mockClear()
    saved.draft.value!.shape = { kind: 'weekly', days: [], hour: 9, minute: 0 }
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)
    expect(api.preview).not.toHaveBeenCalled()

    const creating = setup([], '')
    creating.open(null)
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)
    expect(api.preview).not.toHaveBeenCalled()
  })

  it('loads the history of a saved schedule when its page opens', async () => {
    const { cards, draft, open } = setup()
    open(cards.value[0])
    await flushPromises()
    expect(api.runs).toHaveBeenCalledWith('demo', 'weekly-summary')
    expect(draft.value!.historyLoaded).toBe(true)
  })

  it('runs a saved schedule now and records the run or the failure', async () => {
    const { cards, runNow } = setup()
    await runNow(cards.value[0])
    expect(cards.value[0].lastRun?.reason).toBe('manual')
    api.runNow.mockRejectedValueOnce(new Error('offline'))
    await runNow(cards.value[0])
    expect(cards.value[0].actionError).toBe('offline')
    expect(cards.value[0].running).toBe(false)
  })
})
