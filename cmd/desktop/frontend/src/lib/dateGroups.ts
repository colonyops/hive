import type { InboxItem } from '../types/feed'

// Local-calendar date bucketing for list views. The Activity log and the feed
// both group rows by when something happened; they differ in granularity —
// Activity keeps one bucket per calendar day, the feed collapses anything
// older than yesterday into coarser Slack/email-style tiers (this/last week,
// then this/last month, then everything else). Kept
// framework-free (no Vue) so it is trivially unit-testable, with `now`
// injectable so tests don't depend on the wall clock.

const DAY_MS = 24 * 60 * 60 * 1000

// dayKey identifies a *local* calendar day: two timestamps share a key iff
// they fall on the same day in the user's timezone. Never bucket by raw epoch
// distance — that straddles local midnight and DST-shortened days.
export function dayKey(date: Date): string {
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`
}

function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

// Weeks start Monday, so on a Monday "Last week" is the whole preceding
// Mon–Sun block rather than a two-day sliver.
function startOfWeek(date: Date): Date {
  const start = startOfDay(date)
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7))
  return start
}

// Round rather than floor: DST makes some local days 23 or 25 hours long.
function daysBetween(from: Date, to: Date): number {
  return Math.round((startOfDay(to).getTime() - startOfDay(from).getTime()) / DAY_MS)
}

function weeksBetween(from: Date, to: Date): number {
  return Math.round((startOfWeek(to).getTime() - startOfWeek(from).getTime()) / (7 * DAY_MS))
}

// Calendar months, so December → January counts as one month apart.
function monthsBetween(from: Date, to: Date): number {
  return (to.getFullYear() - from.getFullYear()) * 12 + (to.getMonth() - from.getMonth())
}

export type DateBucket = 'today' | 'yesterday' | 'this-week' | 'last-week' | 'this-month' | 'last-month' | 'older'

const BUCKET_LABELS: Record<DateBucket, string> = {
  today: 'Today',
  yesterday: 'Yesterday',
  'this-week': 'This week',
  'last-week': 'Last week',
  'this-month': 'This month',
  'last-month': 'Last month',
  older: 'Older',
}

// dateBucket tiers a timestamp against `now`, coarsening as it goes back. The
// checks run newest-first and each one only sees what the ones above rejected,
// which is what keeps the tiers monotonic in time even where their windows
// overlap: on the 1st of a month everything in "this month" has already been
// claimed by the week tiers, so that tier is simply empty rather than out of
// order. A future timestamp (clock skew between the app and a source) reads as
// Today rather than tumbling into "Older" via a next-week comparison.
export function dateBucket(timestamp: number, now: Date = new Date()): DateBucket {
  const date = new Date(timestamp)
  const days = daysBetween(date, now)
  if (days <= 0) return 'today'
  if (days === 1) return 'yesterday'
  switch (weeksBetween(date, now)) {
    case 0:
      return 'this-week'
    case 1:
      return 'last-week'
  }
  switch (monthsBetween(date, now)) {
    case 0:
      return 'this-month'
    case 1:
      return 'last-month'
    default:
      return 'older'
  }
}

export interface DateGroup {
  key: DateBucket
  label: string
  items: InboxItem[]
}

// groupItemsByDate buckets an already time-ordered list (newest- or
// oldest-first) into consecutive runs, preserving the input order within each
// group. A bucket appears at most once only while the list is sorted by the
// same timestamp this groups on — lastEventAt, which is what the newest and
// oldest sorts order by.
export function groupItemsByDate(items: InboxItem[], now: Date = new Date()): DateGroup[] {
  const groups: DateGroup[] = []
  let current: DateGroup | null = null
  for (const item of items) {
    const key = dateBucket(item.lastEventAt, now)
    if (!current || current.key !== key) {
      current = { key, label: BUCKET_LABELS[key], items: [] }
      groups.push(current)
    }
    current.items.push(item)
  }
  return groups
}
