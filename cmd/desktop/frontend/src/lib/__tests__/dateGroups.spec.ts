import { describe, expect, it } from 'vitest'
import { dateBucket, dayKey, groupItemsByDate } from '../dateGroups'
import type { InboxItem } from '../../types/feed'

// 2026-07-22 is a Wednesday, so "this week" is Mon 20th onward and "last week"
// is Mon 13th – Sun 19th. All fixtures are local times: the buckets are
// calendar-based, so a UTC-built date would move the boundaries per timezone.
const now = new Date(2026, 6, 22, 14, 30, 0)

function at(month: number, day: number, hour: number, minute = 0, year = 2026): number {
  return new Date(year, month, day, hour, minute).getTime()
}

function item(id: number, lastEventAt: number): InboxItem {
  return { id, profileId: 'triage', sourceKind: 'github', sourceScope: 'acme/app', externalId: `pr-${id}`, title: `Item ${id}`, url: '', payload: {}, revision: 1, unread: false, lifecycle: 'active', firstSeenAt: 0, lastEventAt }
}

describe('dayKey', () => {
  it('is shared by timestamps on the same local day and differs across local midnight', () => {
    expect(dayKey(new Date(2026, 6, 22, 0, 0, 0))).toBe(dayKey(new Date(2026, 6, 22, 23, 59, 59)))
    expect(dayKey(new Date(2026, 6, 22, 23, 59, 59))).not.toBe(dayKey(new Date(2026, 6, 23, 0, 0, 0)))
  })
})

describe('dateBucket', () => {
  it('tiers timestamps relative to now', () => {
    expect(dateBucket(at(6, 22, 14), now)).toBe('today')
    expect(dateBucket(at(6, 21, 23), now)).toBe('yesterday')
    expect(dateBucket(at(6, 20, 9), now)).toBe('this-week')
    expect(dateBucket(at(6, 19, 22), now)).toBe('last-week')
    expect(dateBucket(at(6, 13, 0, 30), now)).toBe('last-week')
    expect(dateBucket(at(6, 12, 23, 30), now)).toBe('this-month')
    expect(dateBucket(at(6, 1, 0, 30), now)).toBe('this-month')
    expect(dateBucket(at(5, 30, 23, 30), now)).toBe('last-month')
    expect(dateBucket(at(5, 1, 8), now)).toBe('last-month')
    expect(dateBucket(at(4, 31, 23, 30), now)).toBe('older')
  })

  it('counts month distance by calendar month across a year boundary', () => {
    const january = new Date(2026, 0, 20, 9, 0, 0) // Tuesday
    expect(dateBucket(at(0, 2, 9), january)).toBe('this-month')
    expect(dateBucket(at(11, 20, 9, 0, 2025), january)).toBe('last-month')
    expect(dateBucket(at(10, 20, 9, 0, 2025), january)).toBe('older')
  })

  it('leaves this-month empty when the week tiers already cover the month', () => {
    // Wednesday the 1st: everything in July is inside this or last week, so the
    // month tiers pick up where the weeks stop rather than reordering.
    const firstOfMonth = new Date(2026, 6, 1, 9, 0, 0)
    expect(dateBucket(at(5, 29, 9), firstOfMonth)).toBe('this-week') // Mon Jun 29
    expect(dateBucket(at(5, 24, 9), firstOfMonth)).toBe('last-week')
    expect(dateBucket(at(5, 10, 9), firstOfMonth)).toBe('last-month')
    expect(dateBucket(at(4, 10, 9), firstOfMonth)).toBe('older')
  })

  it('buckets by local calendar day, not by elapsed hours', () => {
    // 40 minutes earlier, but on the far side of local midnight: Yesterday.
    expect(dateBucket(at(6, 21, 23, 40), new Date(2026, 6, 22, 0, 20, 0))).toBe('yesterday')
    // 20 hours earlier, still the same calendar day: Today.
    expect(dateBucket(at(6, 22, 2), new Date(2026, 6, 22, 22, 0, 0))).toBe('today')
  })

  it('keeps Monday sane: yesterday wins over the week Sunday belongs to', () => {
    const monday = new Date(2026, 6, 20, 9, 0, 0)
    expect(dateBucket(at(6, 19, 20), monday)).toBe('yesterday') // Sunday
    expect(dateBucket(at(6, 18, 20), monday)).toBe('last-week') // Saturday
    expect(dateBucket(at(6, 12, 20), monday)).toBe('this-month')
    expect(dateBucket(at(5, 12, 20), monday)).toBe('last-month')
    expect(dateBucket(at(4, 12, 20), monday)).toBe('older')
  })

  it('reads a future timestamp as today rather than falling through to older', () => {
    expect(dateBucket(at(6, 26, 9), now)).toBe('today')
  })
})

describe('groupItemsByDate', () => {
  it('splits a newest-first list into labeled tiers', () => {
    const items = [
      item(7, at(6, 22, 11)),
      item(6, at(6, 22, 8)),
      item(5, at(6, 21, 17)),
      item(4, at(6, 20, 10)),
      item(3, at(6, 15, 10)),
      item(2, at(6, 3, 10)),
      item(1, at(5, 3, 10)),
      item(0, at(2, 3, 10)),
    ]
    const groups = groupItemsByDate(items, now)
    expect(groups.map((g) => g.label)).toEqual(['Today', 'Yesterday', 'This week', 'Last week', 'This month', 'Last month', 'Older'])
    expect(groups.map((g) => g.items.map((i) => i.id))).toEqual([[7, 6], [5], [4], [3], [2], [1], [0]])
  })

  it('reverses the tiers for an oldest-first list without repeating a bucket', () => {
    const items = [item(1, at(2, 3, 10)), item(2, at(6, 21, 17)), item(3, at(6, 22, 8))]
    expect(groupItemsByDate(items, now).map((g) => g.label)).toEqual(['Older', 'Yesterday', 'Today'])
  })

  it('returns no groups for an empty list', () => {
    expect(groupItemsByDate([], now)).toEqual([])
  })
})
