import { describe, expect, it } from 'vitest'
import type { Event as ActivityEvent } from '../../../bindings/github.com/colonyops/hive/internal/desktop/activity/models'
import {
  eventStyleKey,
  groupEventsByDay,
  matchesFilter,
  matchesSearch,
  timeLabel,
} from '../activityPresentation'

function event(partial: Partial<ActivityEvent> & { id: number }): ActivityEvent {
  return {
    createdAt: 0,
    category: 'system',
    severity: 'info',
    title: 'event',
    ...partial,
  }
}

describe('eventStyleKey', () => {
  it('resolves error severity ahead of category', () => {
    expect(eventStyleKey(event({ id: 1, category: 'refresh', severity: 'error' }))).toBe('error')
  })

  it('resolves the category for non-error events', () => {
    expect(eventStyleKey(event({ id: 1, category: 'auto_action', severity: 'auto' }))).toBe('auto_action')
    expect(eventStyleKey(event({ id: 2, category: 'session', severity: 'success' }))).toBe('session')
    expect(eventStyleKey(event({ id: 3, category: 'action', severity: 'success' }))).toBe('action')
    expect(eventStyleKey(event({ id: 4, category: 'config', severity: 'info' }))).toBe('config')
    expect(eventStyleKey(event({ id: 5, category: 'refresh', severity: 'info' }))).toBe('refresh')
  })

  it('falls back to system for unknown categories', () => {
    expect(eventStyleKey(event({ id: 1, category: 'whatever', severity: 'info' }))).toBe('system')
  })
})

describe('matchesFilter', () => {
  const refresh = event({ id: 1, category: 'refresh', severity: 'info' })
  const failedRefresh = event({ id: 2, category: 'refresh', severity: 'error' })
  const session = event({ id: 3, category: 'session', severity: 'success' })
  const auto = event({ id: 4, category: 'auto_action', severity: 'auto' })

  it('matches everything under "all"', () => {
    for (const e of [refresh, failedRefresh, session, auto]) {
      expect(matchesFilter(e, 'all')).toBe(true)
    }
  })

  it('matches by category for session/refresh/auto_action', () => {
    expect(matchesFilter(session, 'session')).toBe(true)
    expect(matchesFilter(refresh, 'refresh')).toBe(true)
    expect(matchesFilter(auto, 'auto_action')).toBe(true)
    expect(matchesFilter(refresh, 'session')).toBe(false)
  })

  it('matches errors by severity, not category', () => {
    expect(matchesFilter(failedRefresh, 'error')).toBe(true)
    expect(matchesFilter(refresh, 'error')).toBe(false)
  })
})

describe('matchesSearch', () => {
  const e = event({ id: 1, title: 'Refreshed github:hive/core', body: '12 items updated', source: 'github:hive/core' })

  it('is case-insensitive across title, body, and source', () => {
    expect(matchesSearch(e, 'HIVE')).toBe(true)
    expect(matchesSearch(e, '12 items')).toBe(true)
    expect(matchesSearch(e, 'core')).toBe(true)
    expect(matchesSearch(e, 'sentry')).toBe(false)
  })

  it('treats blank queries as matching', () => {
    expect(matchesSearch(e, '   ')).toBe(true)
  })
})

describe('groupEventsByDay', () => {
  const now = new Date(2026, 6, 20, 14, 30, 0) // 2026-07-20 14:30 local
  const ms = (d: Date) => d.getTime()

  it('labels the two most recent days Today and Yesterday and keeps input order', () => {
    const events = [
      event({ id: 3, createdAt: ms(new Date(2026, 6, 20, 14, 0)) }),
      event({ id: 2, createdAt: ms(new Date(2026, 6, 20, 9, 0)) }),
      event({ id: 1, createdAt: ms(new Date(2026, 6, 19, 18, 0)) }),
      event({ id: 0, createdAt: ms(new Date(2026, 6, 10, 8, 0)) }),
    ]
    const groups = groupEventsByDay(events, now)
    expect(groups.map((g) => g.label)).toEqual(['Today', 'Yesterday', expect.stringContaining('2026')])
    expect(groups[0].events.map((e) => e.id)).toEqual([3, 2])
    expect(groups[1].events.map((e) => e.id)).toEqual([1])
    expect(groups[2].events.map((e) => e.id)).toEqual([0])
  })

  it('returns no groups for an empty list', () => {
    expect(groupEventsByDay([], now)).toEqual([])
  })
})

describe('timeLabel', () => {
  it('formats a 24-hour HH:MM:SS stamp', () => {
    const t = timeLabel(new Date(2026, 6, 20, 14, 32, 7).getTime())
    expect(t).toMatch(/^\d{2}:\d{2}:\d{2}$/)
  })
})
