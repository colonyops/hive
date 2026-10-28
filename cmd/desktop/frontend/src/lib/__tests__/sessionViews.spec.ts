import { describe, expect, it } from 'vitest'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import { matchesSessionFilter, type SessionCondition, type SessionFilterContext } from '../sessionViews'

const now = Date.parse('2026-10-28T12:00:00Z')

function row(overrides: Partial<TerminalSessionRow> = {}): TerminalSessionRow {
  return {
    id: '1',
    name: 'parser',
    slug: 'parser-1',
    repo: 'github.com/acme/site',
    repoIdentity: 'github.com/acme/site',
    state: 'active',
    canvasOwner: '',
    tags: ['bug', 'wip'],
    createdAt: '2026-10-20T12:00:00Z',
    ...overrides,
  }
}

const ctx: SessionFilterContext = { rowRunning: (r) => r.id === '1', statusesLoaded: true, now }

const matches = (conditions: SessionCondition[], match: 'all' | 'any' = 'all', r = row(), c = ctx) =>
  matchesSessionFilter(r, { match, conditions }, c)

describe('matchesSessionFilter', () => {
  it('passes every row through an empty filter', () => {
    expect(matches([])).toBe(true)
  })

  it('matches tags case-insensitively with any, all, and none', () => {
    expect(matches([{ field: 'tag', op: 'any', values: ['BUG', 'docs'] }])).toBe(true)
    expect(matches([{ field: 'tag', op: 'all', values: ['bug', 'docs'] }])).toBe(false)
    expect(matches([{ field: 'tag', op: 'none', values: ['docs'] }])).toBe(true)
    expect(matches([{ field: 'tag', op: 'none', values: ['wip'] }])).toBe(false)
  })

  it('matches repositories by identity', () => {
    expect(matches([{ field: 'repo', op: 'any', values: ['github.com/acme/site'] }])).toBe(true)
    expect(matches([{ field: 'repo', op: 'none', values: ['github.com/acme/site'] }])).toBe(false)
  })

  it('matches text against name, slug, and tags', () => {
    expect(matches([{ field: 'text', op: 'contains', value: 'pars' }])).toBe(true)
    expect(matches([{ field: 'text', op: 'contains', value: 'WIP' }])).toBe(true)
    expect(matches([{ field: 'text', op: 'excludes', value: 'pars' }])).toBe(false)
  })

  it('ignores a status condition until statuses have loaded', () => {
    const stopped: SessionCondition[] = [{ field: 'status', op: 'is', value: 'stopped' }]
    expect(matches(stopped)).toBe(false)
    expect(matches(stopped, 'all', row(), { ...ctx, statusesLoaded: false })).toBe(true)
  })

  it('compares age in days and keeps rows with no start time', () => {
    expect(matches([{ field: 'age', op: 'older', days: 7 }])).toBe(true)
    expect(matches([{ field: 'age', op: 'newer', days: 7 }])).toBe(false)
    expect(matches([{ field: 'age', op: 'older', days: 7 }], 'all', row({ createdAt: '' }))).toBe(true)
  })

  it('combines conditions with all or any', () => {
    const conditions: SessionCondition[] = [
      { field: 'tag', op: 'any', values: ['docs'] },
      { field: 'text', op: 'contains', value: 'parser' },
    ]
    expect(matches(conditions, 'all')).toBe(false)
    expect(matches(conditions, 'any')).toBe(true)
  })
})

describe('unknown conditions', () => {
  it('passes a field this build does not know', () => {
    const unknown = { field: 'future', op: 'is', value: 'x' } as unknown as SessionCondition
    expect(matches([unknown])).toBe(true)
  })
})
