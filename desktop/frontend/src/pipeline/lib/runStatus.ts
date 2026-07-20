// Shared node run-status classification (idle / running / done / error) —
// FlowsCanvas (per-card status line + border glow) and FlowsView (the
// bottom status strip's aggregate "N ok/running/idle/error" counts) both
// classify a node's status through this one module so the aggregate counts
// can never disagree with what an individual card shows.
//
// "running" has no real per-node signal yet — node_run rows are only ever
// written for a *completed* pump (see internal/desktop/pipeline), so there is
// no backend concept of "this node is mid-execution" today. Callers pass an
// explicit `running: boolean` (see FlowsCanvas's `runningNodeIds` prop).
// The current runtime records completed node_run rows but does not expose a
// per-node in-flight signal, so callers normally classify nodes as
// idle/ok/error from the latest completed run.
import type { NodeRunRecord } from './wireFlow'

export type RunStatus = 'idle' | 'running' | 'ok' | 'error'

export function classify(run: NodeRunRecord | undefined, running: boolean): RunStatus {
  if (running) return 'running'
  if (!run) return 'idle'
  return run.ok ? 'ok' : 'error'
}

/** The status line's label text for idle, running, ok, and error states. */
export function statusLabel(status: RunStatus, run: NodeRunRecord | undefined): string {
  switch (status) {
    case 'running':
      return 'running…'
    case 'error':
      return `error: ${run?.err || 'error'}`
    case 'ok':
      return run ? `${run.inCount} → ${run.outCount} · ${ageLabel(run.endedAt)}` : 'idle'
    case 'idle':
    default:
      return 'idle'
  }
}

/** The status dot's color token — a CSS color value ready for an inline `background`. */
export function statusColor(status: RunStatus): string {
  switch (status) {
    case 'running':
      return 'var(--color-severity-running)'
    case 'ok':
      return 'var(--color-severity-success)'
    case 'error':
      return 'var(--color-severity-error)'
    case 'idle':
    default:
      return 'var(--color-text-4)'
  }
}

/** Whether the status dot pulses: running and error animate; idle/ok are static. */
export function statusPulses(status: RunStatus): boolean {
  return status === 'running' || status === 'error'
}

// endedAt is stored as Go's time.UnixNano() — convert to ms for Date.now() comparisons.
export function ageLabel(endedAtNano: number): string {
  const ms = Date.now() - endedAtNano / 1e6
  if (ms < 1000) return 'just now'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  return `${h}h ago`
}
