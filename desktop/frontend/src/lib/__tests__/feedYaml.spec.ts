import { describe, expect, it } from 'vitest'
import { feedEntryYaml } from '../feedYaml'

describe('feedEntryYaml', () => {
  it('renders a minimal entry without an id or filters block', () => {
    expect(feedEntryYaml({ id: '', name: 'Team PRs', sources: ['my-prs'], filters: {} })).toBe(
      ['- name: Team PRs', '  sources:', '    - my-prs'].join('\n'),
    )
  })

  it('renders the id first in edit mode and keeps filter group order', () => {
    const yaml = feedEntryYaml({
      id: 'team-prs',
      name: 'Team PRs',
      sources: ['my-prs', 'inbox'],
      filters: { reasons: ['mention'], types: ['pr'], repos: ['acme/*'] },
    })
    expect(yaml).toBe([
      '- id: team-prs',
      '  name: Team PRs',
      '  sources:',
      '    - my-prs',
      '    - inbox',
      '  filters:',
      '    repos:',
      '      - "acme/*"',
      '    types:',
      '      - pr',
      '    reasons:',
      '      - mention',
    ].join('\n'))
  })

  it('quotes glob patterns, including brace globs with commas, without splitting them', () => {
    const yaml = feedEntryYaml({
      id: '',
      name: 'X',
      sources: ['s'],
      filters: { repos: ['acme/{api,web}/**'], exclude_authors: ['*[bot]'] },
    })
    expect(yaml).toContain('- "acme/{api,web}/**"')
    expect(yaml).toContain('- "*[bot]"')
  })

  it('quotes scalars YAML would reparse as another type and shows empty sources explicitly', () => {
    const yaml = feedEntryYaml({ id: '', name: 'true', sources: [], filters: {} })
    expect(yaml).toContain('- name: "true"')
    expect(yaml).toContain('  sources: []')
  })
})
