import { timeLabel } from './activityPresentation'
import { relativeAge, relativeTimeLabel } from './age'
import type { AgentScheduleRun } from './agentWorkspacesClient'

const DAY_MS = 24 * 60 * 60 * 1000
/** Under a minute out, a countdown reads as noise; the row says it is up instead. */
const IMMINENT_MS = 60 * 1000

const REASON_LABELS: Record<string, string> = { due: 'on schedule', catch_up: 'catch-up', manual: 'by hand' }

export function reasonLabel(run: AgentScheduleRun): string {
  return REASON_LABELS[run.reason] ?? run.reason
}

// A run at its scheduled time is the ordinary case, so only the other reasons are said.
export function lastRunLabel(run: AgentScheduleRun, now = Date.now()): string {
  const reason = run.reason === 'due' ? '' : ` · ${reasonLabel(run)}`
  return `Last run ${run.status} ${relativeTimeLabel(run.startedAt, now)}${reason}`
}

/** Empty while the timetable is not the scheduler's yet, which the row's unsaved chip already says. */
export function nextRunLabel(
  card: { disabled: boolean; saved: boolean; edited: boolean; nextRunAt: number | null },
  now = Date.now(),
): string {
  if (card.disabled) return 'paused'
  if (!card.saved || card.edited) return ''
  if (card.nextRunAt === null) return 'not scheduled'
  const delta = card.nextRunAt - now
  if (delta <= IMMINENT_MS) return 'due now'
  // relativeAge measures backwards from its second argument, so the two are
  // swapped to get the same terse form ("2h") for a time still to come.
  if (delta < DAY_MS) return `next in ${relativeAge(now, card.nextRunAt)}`
  return `next ${occurrence(card.nextRunAt)}`
}

export function occurrence(at: number): string {
  return new Date(at).toLocaleString([], {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

// The launcher rows' dot vocabulary: a launch reads as success, a failure as
// an error, and everything else stays neutral chrome.
export function statusDot(status: string): string {
  if (status === 'failed') return 'bg-severity-error'
  if (status === 'launched') return 'bg-severity-success'
  return 'bg-text-4'
}

// A history spanning days needs the day; a run from today does not.
export function runStamp(at: number): string {
  const when = new Date(at)
  if (when.toDateString() === new Date().toDateString()) return timeLabel(at)
  return `${when.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${timeLabel(at)}`
}
