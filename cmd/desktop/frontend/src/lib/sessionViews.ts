import type { TerminalSessionRow } from '../stores/useTerminalSessions'

export type SessionCondition =
  | { field: 'tag'; op: 'any' | 'all' | 'none'; values: readonly string[] }
  | { field: 'repo'; op: 'any' | 'none'; values: readonly string[] }
  | { field: 'status'; op: 'is'; value: 'running' | 'stopped' }
  | { field: 'age'; op: 'older' | 'newer'; days: number }

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
export const RUNNING_VIEW_ID = 'running'

export function emptySessionFilter(): SessionFilter {
  return { match: 'all', conditions: [] }
}

/** The views every install has. They lead the switcher and cannot be edited in place, renamed, or deleted. */
export const BUILT_IN_VIEWS: readonly SavedSessionView[] = [
  { id: ALL_SESSIONS_VIEW_ID, name: 'All sessions', filter: emptySessionFilter() },
  {
    id: RUNNING_VIEW_ID,
    name: 'Running',
    filter: { match: 'all', conditions: [{ field: 'status', op: 'is', value: 'running' }] },
  },
]

export function isBuiltInView(id: string): boolean {
  return BUILT_IN_VIEWS.some((view) => view.id === id)
}

/** Whether the filter reads run status, so the tree has to wait for the first poll before painting. */
export function filterReadsStatus(filter: SessionFilter): boolean {
  return filter.conditions.some((condition) => condition.field === 'status')
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

/**
 * A condition as a GitHub-style search qualifier (`status:running`,
 * `-tag:wip`, `age:>7d`), which is what a chip shows. A leading `-` negates.
 */
export function conditionQualifier(condition: SessionCondition, repoName: (identity: string) => string): string {
  switch (condition.field) {
    case 'tag': {
      const values = condition.values.join(condition.op === 'all' ? '+' : ',') || '…'
      return `${condition.op === 'none' ? '-' : ''}tag:${values}`
    }
    case 'repo':
      return `${condition.op === 'none' ? '-' : ''}repo:${condition.values.map(repoName).join(',') || '…'}`
    case 'status':
      return `status:${condition.value}`
    case 'age':
      return `age:${condition.op === 'older' ? '>' : '<'}${condition.days}d`
    default: {
      const unknown = condition as { field: string; op: string }
      return `${unknown.field}:${unknown.op}?`
    }
  }
}

/** A plain-words reading of a condition, for a chip's tooltip. */
export function describeCondition(condition: SessionCondition, repoName: (identity: string) => string): string {
  switch (condition.field) {
    case 'tag': {
      const op = { any: 'is any of', all: 'has all of', none: 'is none of' }[condition.op]
      return `Tag ${op} ${condition.values.join(', ') || '…'}`
    }
    case 'repo':
      return `Repo ${condition.op === 'any' ? 'is' : 'is not'} ${condition.values.map(repoName).join(', ') || '…'}`
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
