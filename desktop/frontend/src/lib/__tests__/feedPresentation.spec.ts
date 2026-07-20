import { describe, expect, it } from 'vitest'
import { bodySnippet, feedSource, typeLabel } from '../feedPresentation'

describe('feedSource', () => {
  it('resolves items to their source (GitHub today)', () => {
    expect(feedSource({ url: 'https://github.com/colonyops/hive/pull/42' })).toEqual({ key: 'github', label: 'GitHub' })
    expect(feedSource()).toEqual({ key: 'github', label: 'GitHub' })
  })
})

describe('typeLabel', () => {
  it('maps known kinds to human labels', () => {
    expect(typeLabel('PR')).toBe('Pull Request')
    expect(typeLabel('Issue')).toBe('Issue')
  })

  it('falls back to the raw kind, then a generic label', () => {
    expect(typeLabel('Alert')).toBe('Alert')
    expect(typeLabel('')).toBe('Item')
  })
})

describe('bodySnippet', () => {
  it('takes the first non-empty line and strips heading markers', () => {
    expect(bodySnippet('## Summary\n\nMore detail')).toBe('Summary')
  })

  it('strips list, task-list, and blockquote markers', () => {
    expect(bodySnippet('- [ ] do the thing')).toBe('do the thing')
    expect(bodySnippet('> quoted intro')).toBe('quoted intro')
  })

  it('reduces links to their text and drops inline emphasis', () => {
    expect(bodySnippet('see **[the docs](https://example.com)** now')).toBe('see the docs now')
  })

  it('returns an empty string for a blank body', () => {
    expect(bodySnippet('\n\n')).toBe('')
  })
})
