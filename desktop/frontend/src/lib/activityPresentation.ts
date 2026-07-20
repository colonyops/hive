import type { Event as ActivityEvent } from '../../bindings/github.com/colonyops/hive/internal/desktop/activity/models'

// Pure presentation helpers for the Activity view: which filter pills exist,
// how an event matches a filter/search, how events group by day, and the style
// key that drives its icon and accent color. Kept framework-free (no icon
// components, no Vue) so it is trivially unit-testable; ActivityView maps the
// style key to a lucide icon + Tailwind token classes.

export type ActivityFilterId = 'all' | 'session' | 'auto_action' | 'refresh' | 'error'

export interface ActivityFilter {
  id: ActivityFilterId
  label: string
}

// Mirrors the mockup's toolbar pills, in order.
export const ACTIVITY_FILTERS: ActivityFilter[] = [
  { id: 'all', label: 'All' },
  { id: 'session', label: 'Sessions' },
  { id: 'auto_action', label: 'Auto actions' },
  { id: 'refresh', label: 'Refreshes' },
  { id: 'error', label: 'Errors' },
]

// The style key resolves an event to one visual treatment. Severity=error wins
// over category, so a failed refresh reads as an error, matching the design.
export type ActivityStyleKey = 'error' | 'auto_action' | 'refresh' | 'session' | 'action' | 'config' | 'system'

export function eventStyleKey(event: ActivityEvent): ActivityStyleKey {
  if (event.severity === 'error') return 'error'
  switch (event.category) {
    case 'auto_action':
      return 'auto_action'
    case 'refresh':
      return 'refresh'
    case 'session':
      return 'session'
    case 'action':
      return 'action'
    case 'config':
      return 'config'
    default:
      return 'system'
  }
}

export function matchesFilter(event: ActivityEvent, filter: ActivityFilterId): boolean {
  switch (filter) {
    case 'all':
      return true
    case 'error':
      return event.severity === 'error'
    default:
      return event.category === filter
  }
}

export function matchesSearch(event: ActivityEvent, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return (
    event.title.toLowerCase().includes(q) ||
    (event.body ?? '').toLowerCase().includes(q) ||
    (event.source ?? '').toLowerCase().includes(q)
  )
}

export interface ActivityDayGroup {
  key: string
  label: string
  events: ActivityEvent[]
}

// groupEventsByDay buckets already-newest-first events into calendar days,
// labeling the two most recent as Today/Yesterday. `now` is injectable so tests
// don't depend on the wall clock.
export function groupEventsByDay(events: ActivityEvent[], now: Date = new Date()): ActivityDayGroup[] {
  const today = dayKey(now)
  const yesterday = dayKey(new Date(now.getTime() - 24 * 60 * 60 * 1000))

  const groups: ActivityDayGroup[] = []
  let current: ActivityDayGroup | null = null
  for (const event of events) {
    const date = new Date(event.createdAt)
    const key = dayKey(date)
    if (!current || current.key !== key) {
      current = { key, label: dayLabel(key, date, today, yesterday), events: [] }
      groups.push(current)
    }
    current.events.push(event)
  }
  return groups
}

// timeLabel is the right-aligned HH:MM:SS stamp on each row.
export function timeLabel(createdAt: number): string {
  return new Date(createdAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

function dayKey(date: Date): string {
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`
}

function dayLabel(key: string, date: Date, today: string, yesterday: string): string {
  if (key === today) return 'Today'
  if (key === yesterday) return 'Yesterday'
  return date.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' })
}
