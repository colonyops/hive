import { describe, expect, it } from 'vitest'
import { globToRegExp, matchGlob, matches, type Config, type FilterableItem } from '../config'
import githubFilterRuntime from '../runtime'
import type { Msg } from '../../../types'

function item(overrides: Partial<FilterableItem> = {}): FilterableItem {
  return { repo: 'acme/app', author: 'hayden', kind: 'PR', reason: '', labels: [], ...overrides }
}

describe('github-filter glob matcher', () => {
  it('* matches within one path segment only', () => {
    expect(matchGlob('acme/*', 'acme/app')).toBe(true)
    expect(matchGlob('acme/*', 'other/app')).toBe(false)
    expect(matchGlob('acme/*', 'acme/app/sub')).toBe(false)
  })

  it('** matches across segments', () => {
    expect(matchGlob('acme/**', 'acme/app/sub')).toBe(true)
  })

  it('treats [ and ] as literal characters — no character-class support', () => {
    expect(matchGlob('*[bot]', 'dependabot[bot]')).toBe(true)
    expect(matchGlob('dependabot[bot]', 'dependabot[bot]')).toBe(true)
    expect(matchGlob('dependabot[bot]', 'dependabotx')).toBe(false)
  })

  it('globToRegExp anchors the whole string', () => {
    expect(globToRegExp('acme/app').test('acme/app-extra')).toBe(false)
  })
})

describe('github-filter matches() — parity with internal/desktop/feed/filters_test.go', () => {
  // Mirrors TestApplyFilters' fixture (filters_test.go) so the same filter
  // groups produce the same pass/fail sets as the Go implementation.
  const searchPR = item({ repo: 'acme/app', author: 'hayden', kind: 'PR', reason: '', labels: ['bug', 'area/ui'] })
  const searchIssue = item({ repo: 'acme/web', author: 'Mira', kind: 'Issue', reason: '', labels: ['wontfix'] })
  const botPR = item({ repo: 'acme/app', author: 'dependabot[bot]', kind: 'PR', reason: '', labels: [] })
  const notifIssue = item({ repo: 'other/repo', author: '', kind: 'Issue', reason: 'mention', labels: [] })
  const mergedPR = item({ repo: 'acme/app', author: 'koji', kind: 'PR', reason: 'review_requested', labels: ['bug'] })
  const all = [searchPR, searchIssue, botPR, notifIssue, mergedPR]

  function pass(config: Config): FilterableItem[] {
    return all.filter((candidate) => matches(config, candidate))
  }

  it('no filters keeps all', () => {
    expect(pass({})).toEqual(all)
  })

  it('repos include', () => {
    expect(pass({ repos: ['acme/*'] })).toEqual([searchPR, searchIssue, botPR, mergedPR])
  })

  it('exclude wins over include', () => {
    expect(pass({ repos: ['acme/*'], exclude_repos: ['acme/app'] })).toEqual([searchIssue])
  })

  it('authors match case-insensitively', () => {
    expect(pass({ authors: ['MIRA'] })).toEqual([searchIssue])
  })

  it('excludes bot authors via literal brackets', () => {
    expect(pass({ exclude_authors: ['dependabot[bot]'] })).toEqual([searchPR, searchIssue, notifIssue, mergedPR])
  })

  it('excludes bot authors via a wildcard + literal brackets', () => {
    expect(pass({ exclude_authors: ['*[bot]'] })).toEqual([searchPR, searchIssue, notifIssue, mergedPR])
  })

  it('labels OR within the group', () => {
    expect(pass({ labels: ['bug', 'wontfix'] })).toEqual([searchPR, searchIssue, mergedPR])
  })

  it('types matches case-insensitively', () => {
    expect(pass({ types: ['pr'] })).toEqual([searchPR, botPR, mergedPR])
  })

  it('a missing reason excludes search-only items from a reasons filter', () => {
    expect(pass({ reasons: ['mention'] })).toEqual([notifIssue])
  })

  it('groups AND together', () => {
    expect(pass({ repos: ['acme/*'], types: ['pr'], labels: ['bug'] })).toEqual([searchPR, mergedPR])
  })
})

describe('github-filter runtime — 2-port routing', () => {
  function msg(id: string, payload: FilterableItem): Msg {
    return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: payload, SourceKind: 'github', SourceScope: 'acme/app' }
  }

  it('routes a passing item to port 0', async () => {
    const ctx = { config: { repos: ['acme/*'] } as Config, state: {} }
    const m = msg('1', item())
    expect(await githubFilterRuntime.onMsg(m, ctx)).toEqual([m, null])
  })

  it('routes a failing item to port 1', async () => {
    const ctx = { config: { repos: ['other/*'] } as Config, state: {} }
    const m = msg('2', item())
    expect(await githubFilterRuntime.onMsg(m, ctx)).toEqual([null, m])
  })
})
