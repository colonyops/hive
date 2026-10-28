import type { TerminalSessionRow } from '../stores/useTerminalSessions'

export type SessionCondition =
  | { field: 'tag'; op: 'any' | 'all' | 'none'; values: readonly string[] }
  | { field: 'repo'; op: 'any' | 'none'; values: readonly string[] }
  | { field: 'text'; op: 'contains' | 'excludes'; value: string }
  | { field: 'status'; op: 'is'; value: 'running' | 'stopped' }
  | { field: 'age'; op: 'older' | 'newer'; days: number }

export type SessionConditionField = SessionCondition['field']

export interface SessionFilter {
  match: 'all' | 'any'
  conditions: readonly SessionCondition[]
}

export interface SavedSessionView {
  id: string
  name: string
  filter: SessionFilter
}

export interface SessionFilterContext {
  rowRunning: (row: TerminalSessionRow) => boolean
  /** False until the first status poll lands; a status condition passes every row until then. */
  statusesLoaded: boolean
  now: number
}

export const ALL_SESSIONS_VIEW_ID = 'all'

export function emptySessionFilter(): SessionFilter {
  return { match: 'all', conditions: [] }
}

const dayMilliseconds = 24 * 60 * 60 * 1000

function matchesCondition(row: TerminalSessionRow, condition: SessionCondition, ctx: SessionFilterContext): boolean {
  switch (condition.field) {
    case 'tag': {
      if (!condition.values.length) return true
      const tags = new Set((row.tags ?? []).map((tag) => tag.toLowerCase()))
      const wanted = condition.values.map((tag) => tag.toLowerCase())
      if (condition.op === 'all') return wanted.every((tag) => tags.has(tag))
      const hit = wanted.some((tag) => tags.has(tag))
      return condition.op === 'any' ? hit : !hit
    }
    case 'repo': {
      if (!condition.values.length) return true
      const hit = condition.values.includes(row.repoIdentity)
      return condition.op === 'any' ? hit : !hit
    }
    case 'text': {
      const needle = condition.value.trim().toLowerCase()
      if (!needle) return true
      const hit = [row.name, row.slug, ...(row.tags ?? [])].some((text) => text.toLowerCase().includes(needle))
      return condition.op === 'contains' ? hit : !hit
    }
    case 'status':
      if (!ctx.statusesLoaded) return true
      return ctx.rowRunning(row) === (condition.value === 'running')
    case 'age': {
      const created = Date.parse(row.createdAt)
      // A row with no start time is not old or new; it should not vanish from either view.
      if (!Number.isFinite(created)) return true
      const days = (ctx.now - created) / dayMilliseconds
      return condition.op === 'older' ? days >= condition.days : days < condition.days
    }
    default:
      // A field a newer build wrote: pass it rather than hide sessions for a rule this build cannot read.
      return true
  }
}

/** Whether a hive session row passes the filter. An empty filter passes everything. */
export function matchesSessionFilter(
  row: TerminalSessionRow,
  filter: SessionFilter,
  ctx: SessionFilterContext,
): boolean {
  if (!filter.conditions.length) return true
  const test = (condition: SessionCondition) => matchesCondition(row, condition, ctx)
  return filter.match === 'all' ? filter.conditions.every(test) : filter.conditions.some(test)
}

// structuredClone throws on the reactive proxies a stored filter arrives in.
export function cloneSessionFilter(filter: SessionFilter): SessionFilter {
  return JSON.parse(JSON.stringify(filter)) as SessionFilter
}

export function sameSessionFilter(a: SessionFilter, b: SessionFilter): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

/** A short, human reading of a condition, as its chip shows it. */
export function describeCondition(condition: SessionCondition, repoName: (identity: string) => string): string {
  switch (condition.field) {
    case 'tag': {
      const op = { any: 'is any of', all: 'has all of', none: 'is none of' }[condition.op]
      return `Tag ${op} ${condition.values.join(', ') || '…'}`
    }
    case 'repo':
      return `Repo ${condition.op === 'any' ? 'is' : 'is not'} ${condition.values.map(repoName).join(', ') || '…'}`
    case 'text':
      return `${condition.op === 'contains' ? 'Contains' : 'Excludes'} “${condition.value}”`
    case 'status':
      return `Status is ${condition.value}`
    case 'age':
      return `${condition.op === 'older' ? 'Older' : 'Newer'} than ${condition.days}d`
    default: {
      const unknown = condition as { field: string; op: string }
      return `${unknown.field} ${unknown.op} (unsupported)`
    }
  }
}
