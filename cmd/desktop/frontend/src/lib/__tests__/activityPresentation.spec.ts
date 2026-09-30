import { describe, expect, it } from 'vitest'
import type { Event as ActivityEvent } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/activity/models'
import {
  activityLinks,
  eventStyleKey,
  filterCounts,
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

describe('activityLinks', () => {
  it('decodes external and internal destinations from the generic metadata namespace', () => {
    expect(activityLinks(event({
      id: 1,
      metadata: {
        'link.url': 'https://github.com/acme/api/pull/12',
        'link.item.profileId': 'triage',
        'link.item.sourceKind': 'github',
        'link.item.sourceScope': 'acme/api',
        'link.item.externalId': 'acme/api#12',
      },
    }))).toEqual({
      url: 'https://github.com/acme/api/pull/12',
      item: { profileId: 'triage', sourceKind: 'github', sourceScope: 'acme/api', externalId: 'acme/api#12' },
    })
  })

  it('leaves an event with no link inert', () => {
    expect(activityLinks(event({ id: 1 }))).toEqual({ url: '', item: null })
  })
})

describe('eventStyleKey', () => {
  it('resolves error severity ahead of category', () => {
    expect(eventStyleKey(event({ id: 1, category: 'refresh', severity: 'error' }))).toBe('error')
  })

  it('gives auto-actions their own treatment', () => {
    expect(eventStyleKey(event({ id: 1, category: 'auto_action', severity: 'auto' }))).toBe('auto_action')
  })

  // Hue is severity only, so every category that is neither a failure nor the
  // app acting on its own resolves to the same quiet row.
  it('resolves every other category to neutral', () => {
    expect(eventStyleKey(event({ id: 1, category: 'session', severity: 'success' }))).toBe('neutral')
    expect(eventStyleKey(event({ id: 2, category: 'action', severity: 'success' }))).toBe('neutral')
    expect(eventStyleKey(event({ id: 3, category: 'config', severity: 'info' }))).toBe('neutral')
    expect(eventStyleKey(event({ id: 4, category: 'refresh', severity: 'info' }))).toBe('neutral')
    expect(eventStyleKey(event({ id: 5, category: 'whatever', severity: 'info' }))).toBe('neutral')
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

  it('marks Today/Yesterday relative with a short date, absolute days not', () => {
    const events = [
      event({ id: 1, createdAt: ms(new Date(2026, 6, 20, 14, 0)) }),
      event({ id: 0, createdAt: ms(new Date(2026, 6, 10, 8, 0)) }),
    ]
    const [today, older] = groupEventsByDay(events, now)
    expect(today.isRelative).toBe(true)
    expect(today.dateLabel).toContain('20')
    expect(older.isRelative).toBe(false)
  })

  it('returns no groups for an empty list', () => {
    expect(groupEventsByDay([], now)).toEqual([])
  })
})

describe('filterCounts', () => {
  it('counts each filter, with All as the total and errors by severity', () => {
    const events = [
      event({ id: 1, category: 'refresh', severity: 'info' }),
      event({ id: 2, category: 'refresh', severity: 'error' }),
      event({ id: 3, category: 'session', severity: 'success' }),
      event({ id: 4, category: 'auto_action', severity: 'auto' }),
    ]
    expect(filterCounts(events)).toEqual({ all: 4, session: 1, auto_action: 1, refresh: 2, error: 1 })
  })

  it('is all-zero for an empty list', () => {
    expect(filterCounts([])).toEqual({ all: 0, session: 0, auto_action: 0, refresh: 0, error: 0 })
  })
})

describe('timeLabel', () => {
  it('formats a 24-hour HH:MM:SS stamp', () => {
    const t = timeLabel(new Date(2026, 6, 20, 14, 32, 7).getTime())
    expect(t).toMatch(/^\d{2}:\d{2}:\d{2}$/)
  })
})
