import { describe, expect, it } from 'vitest'
import { lastRunLabel, nextRunLabel, reasonLabel, statusDot } from '../scheduleRuns'
import type { AgentScheduleRun } from '../agentWorkspacesClient'

const NOW = Date.UTC(2026, 9, 2, 12)
const HOUR = 60 * 60 * 1000

function run(overrides: Partial<AgentScheduleRun> = {}): AgentScheduleRun {
  return { id: 1, startedAt: NOW - 2 * HOUR, reason: 'due', status: 'launched', missed: 0, error: '', ...overrides }
}

function card(overrides: Partial<Parameters<typeof nextRunLabel>[0]> = {}) {
  return { disabled: false, saved: true, edited: false, nextRunAt: NOW + 2 * HOUR, ...overrides }
}

describe('nextRunLabel', () => {
  it('says paused, nothing while unsaved, and not scheduled without a time', () => {
    expect(nextRunLabel(card({ disabled: true }), NOW)).toBe('paused')
    expect(nextRunLabel(card({ saved: false }), NOW)).toBe('')
    expect(nextRunLabel(card({ edited: true }), NOW)).toBe('')
    expect(nextRunLabel(card({ nextRunAt: null }), NOW)).toBe('not scheduled')
  })

  it('counts down within a day and calls a run under a minute out due', () => {
    expect(nextRunLabel(card(), NOW)).toBe('next in 2h')
    expect(nextRunLabel(card({ nextRunAt: NOW + 30_000 }), NOW)).toBe('due now')
    expect(nextRunLabel(card({ nextRunAt: NOW + 48 * HOUR }), NOW)).toMatch(/^next /)
  })
})

describe('run labels', () => {
  it('names only the reasons that are not the ordinary scheduled run', () => {
    expect(lastRunLabel(run(), NOW)).toBe('Last run launched 2 hours ago')
    expect(lastRunLabel(run({ reason: 'catch_up' }), NOW)).toBe('Last run launched 2 hours ago · catch-up')
    expect(reasonLabel(run({ reason: 'manual' }))).toBe('by hand')
  })

  it('colors a failure as an error and a launch as success', () => {
    expect(statusDot('failed')).toBe('bg-severity-error')
    expect(statusDot('launched')).toBe('bg-severity-success')
    expect(statusDot('skipped')).toBe('bg-text-4')
  })
})
