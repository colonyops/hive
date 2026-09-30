import { describe, expect, it } from 'vitest'
import { fuzzyMatch, highlightSegments, rankRepositories, repoDisplayName, toChoices } from '../repositories'

describe('repoDisplayName', () => {
  it('reduces every remote spelling to owner/name', () => {
    expect(repoDisplayName('https://github.com/hay-kot/hive-desktop.git')).toBe('hay-kot/hive-desktop')
    expect(repoDisplayName('git@github.com:hay-kot/hive-desktop.git')).toBe('hay-kot/hive-desktop')
    expect(repoDisplayName('https://github.com/hay-kot/hive-desktop')).toBe('hay-kot/hive-desktop')
    expect(repoDisplayName('/Users/hayden/code/acme/site')).toBe('acme/site')
  })

  it('returns empty for an empty remote', () => {
    expect(repoDisplayName('')).toBe('')
  })
})

describe('toChoices', () => {
  it('labels by owner/name and keeps a name only when it adds something', () => {
    expect(toChoices([
      { name: 'hive-desktop', repository: 'https://github.com/hay-kot/hive-desktop.git' },
      { name: 'fix-crash', repository: 'https://github.com/acme/site.git' },
    ])).toEqual([
      { remote: 'https://github.com/hay-kot/hive-desktop.git', label: 'hay-kot/hive-desktop', hint: '' },
      { remote: 'https://github.com/acme/site.git', label: 'acme/site', hint: 'fix-crash' },
    ])
  })

  it('drops entries without a remote and tolerates a null list', () => {
    expect(toChoices([{ name: 'nope', repository: '  ' }])).toEqual([])
    expect(toChoices(null)).toEqual([])
  })
})

describe('fuzzyMatch', () => {
  it('matches a subsequence and reports the positions', () => {
    const match = fuzzyMatch('hay-kot/hive-desktop', 'hive')
    expect(match?.indices).toEqual([8, 9, 10, 11])
  })

  it('prefers a contiguous run over an earlier scattered one', () => {
    // Greedy from index 0 would match h(0) i(9) v(10) e(11); restarting at the
    // second "h" finds the whole word.
    expect(fuzzyMatch('hay-kot/hive', 'hive')?.indices).toEqual([8, 9, 10, 11])
  })

  it('matches initials across segment boundaries', () => {
    expect(fuzzyMatch('hay-kot/hive-desktop', 'hd')).not.toBeNull()
  })

  it('returns null when a character is missing', () => {
    expect(fuzzyMatch('hay-kot/hive-desktop', 'zz')).toBeNull()
  })

  it('treats an empty query as a match with no positions', () => {
    expect(fuzzyMatch('anything', '')).toEqual({ score: 0, indices: [] })
  })
})

describe('rankRepositories', () => {
  const choices = toChoices([
    { name: 'site', repository: 'https://github.com/acme/site.git' },
    { name: 'hive-desktop', repository: 'https://github.com/hay-kot/hive-desktop.git' },
    { name: 'hive', repository: 'https://github.com/hay-kot/hive.git' },
  ])

  it('keeps backend order for an empty query', () => {
    expect(rankRepositories(choices, '').map((r) => r.label)).toEqual(['acme/site', 'hay-kot/hive-desktop', 'hay-kot/hive'])
  })

  it('lifts the selected repository to the top of an unfiltered list', () => {
    expect(rankRepositories(choices, '', 'https://github.com/hay-kot/hive.git').map((r) => r.label))
      .toEqual(['hay-kot/hive', 'acme/site', 'hay-kot/hive-desktop'])
  })

  it('ranks a fuzzy query and drops non-matches', () => {
    const ranked = rankRepositories(choices, 'hivedesk')
    expect(ranked.map((r) => r.label)).toEqual(['hay-kot/hive-desktop'])
  })

  it('scores the closer name first', () => {
    expect(rankRepositories(choices, 'hive')[0].label).toBe('hay-kot/hive')
  })

  it('matches on the remote when the label does not, without outranking a label hit', () => {
    const ranked = rankRepositories(choices, 'github.com/acme')
    expect(ranked.map((r) => r.label)).toEqual(['acme/site'])
    expect(ranked[0].indices).toEqual([])
  })

  it('finds a session name that the owner/name label hides', () => {
    const withHint = toChoices([{ name: 'fix-crash', repository: 'https://github.com/acme/site.git' }])
    expect(rankRepositories(withHint, 'crash').map((r) => r.label)).toEqual(['acme/site'])
  })
})

describe('highlightSegments', () => {
  it('splits a label into alternating matched runs', () => {
    expect(highlightSegments('hive', [0, 1])).toEqual([
      { text: 'hi', matched: true },
      { text: 've', matched: false },
    ])
  })

  it('returns one plain segment when nothing matched', () => {
    expect(highlightSegments('hive', [])).toEqual([{ text: 'hive', matched: false }])
  })
})
