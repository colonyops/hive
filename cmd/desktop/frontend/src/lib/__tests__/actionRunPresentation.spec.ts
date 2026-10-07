import { describe, expect, it } from 'vitest'
import type { ActionRunSummary } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { formatDuration, groupByAttempt, logText, matchesRun, runDurationMs } from '../actionRunPresentation'

function run(overrides: Partial<ActionRunSummary> = {}): ActionRunSummary {
  return {
    id: 1,
    actionId: 'deploy',
    label: 'Deploy',
    status: 'done',
    attempts: 1,
    lane: 'manual',
    rerun: false,
    key: 'item-1',
    createdAt: 1_000,
    ...overrides,
  }
}

describe('actionRunPresentation', () => {
  it('measures finished runs to their finish and running ones to now', () => {
    expect(runDurationMs(run({ startedAt: 1_000, finishedAt: 3_500 }))).toBe(2_500)
    expect(runDurationMs(run({ status: 'running', startedAt: 1_000 }), 4_000)).toBe(3_000)
    expect(runDurationMs(run({ status: 'pending' }))).toBeNull()
    expect(runDurationMs(run({ startedAt: 1_000 }))).toBeNull()
  })

  it('formats durations at the scale they are read', () => {
    expect(formatDuration(null)).toBe('—')
    expect(formatDuration(420)).toBe('420ms')
    expect(formatDuration(4_250)).toBe('4.3s')
    expect(formatDuration(42_000)).toBe('42s')
    expect(formatDuration(184_000)).toBe('3m 04s')
    expect(formatDuration(3_720_000)).toBe('1h 02m')
  })

  it('groups lines by attempt and numbers them across the whole log', () => {
    const groups = groupByAttempt([
      { id: 5, attempt: 1, stream: 'system', text: 'Started', at: 1 },
      { id: 6, attempt: 1, stream: 'stdout', text: 'one', at: 1 },
      { id: 9, attempt: 2, stream: 'stderr', text: 'two', at: 2 },
    ])
    expect(groups.map((group) => group.attempt)).toEqual([1, 2])
    expect(groups[1]?.lines[0]).toMatchObject({ n: 3, text: 'two' })
  })

  it('copies system lines as comments', () => {
    expect(
      logText([
        { id: 1, attempt: 1, stream: 'system', text: '$ make', at: 1 },
        { id: 2, attempt: 1, stream: 'stdout', text: 'ok', at: 1 },
      ]),
    ).toBe('# $ make\nok')
  })

  it('filters by state and searches labels, items and errors', () => {
    expect(matchesRun(run({ status: 'running' }), 'active', '')).toBe(true)
    expect(matchesRun(run(), 'active', '')).toBe(false)
    expect(matchesRun(run({ status: 'failed' }), 'failed', '')).toBe(true)
    expect(matchesRun(run({ itemTitle: 'Fix the login page' }), 'all', 'login')).toBe(true)
    expect(matchesRun(run({ error: 'exit status 3' }), 'all', 'status 3')).toBe(true)
    expect(matchesRun(run(), 'all', 'nothing')).toBe(false)
  })
})
