import type {
  ActionRunLogLine,
  ActionRunSummary,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

export type RunFilter = 'all' | 'active' | 'failed'

export const RUN_FILTERS: { value: RunFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'active', label: 'Running' },
  { value: 'failed', label: 'Failed' },
]

export function runIsActive(status: string): boolean {
  return status === 'pending' || status === 'running'
}

export function runStatusLabel(status: string): string {
  switch (status) {
    case 'pending':
      return 'Queued'
    case 'running':
      return 'Running'
    case 'done':
      return 'Succeeded'
    case 'failed':
      return 'Failed'
    case 'cancelled':
      return 'Cancelled'
    default:
      return status
  }
}

// A queued run has not started, so it has no duration yet. A finished run
// without a recorded finish (one from before finish times were kept) has none
// either, rather than one that keeps growing.
export function runDurationMs(run: ActionRunSummary, now = Date.now()): number | null {
  const started = run.startedAt || 0
  if (!started) return null
  if (run.finishedAt) return Math.max(0, run.finishedAt - started)
  return runIsActive(run.status) ? Math.max(0, now - started) : null
}

export function formatDuration(ms: number | null): string {
  if (ms === null) return '—'
  if (ms < 1000) return `${Math.round(ms)}ms`
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${(ms / 1000).toFixed(seconds < 10 ? 1 : 0)}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ${String(seconds % 60).padStart(2, '0')}s`
  return `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, '0')}m`
}

export function matchesRun(run: ActionRunSummary, filter: RunFilter, search: string): boolean {
  if (filter === 'active' && !runIsActive(run.status)) return false
  if (filter === 'failed' && run.status !== 'failed') return false
  const query = search.trim().toLowerCase()
  if (!query) return true
  return [run.label, run.actionId, run.itemTitle, run.key, run.error, String(run.id)]
    .filter(Boolean)
    .some((field) => String(field).toLowerCase().includes(query))
}

export interface NumberedLine extends ActionRunLogLine {
  n: number
}

export interface AttemptGroup {
  attempt: number
  lines: NumberedLine[]
}

// Lines arrive in id order, which is attempt order, so a group starts each
// time the attempt changes.
export function groupByAttempt(lines: readonly ActionRunLogLine[]): AttemptGroup[] {
  const groups: AttemptGroup[] = []
  lines.forEach((line, index) => {
    let group = groups[groups.length - 1]
    if (!group || group.attempt !== line.attempt) {
      group = { attempt: line.attempt, lines: [] }
      groups.push(group)
    }
    group.lines.push({ ...line, n: index + 1 })
  })
  return groups
}

export function logText(lines: readonly ActionRunLogLine[]): string {
  return lines.map((line) => (line.stream === 'system' ? `# ${line.text}` : line.text)).join('\n')
}

export function laneLabel(lane: string): string {
  return lane === 'manual' ? 'Manual' : 'Flow'
}
