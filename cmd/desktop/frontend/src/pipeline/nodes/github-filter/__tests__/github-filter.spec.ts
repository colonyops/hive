// The filter *rule* — glob dialect, group composition, exclude precedence —
// is Go's, and its tests live with it in internal/app/runtime/filter_test.go
// and the engine's parity fixtures. What is left here is the editor's half:
// the drawer's live validity check.

import { describe, expect, it } from 'vitest'
import { defaults, outputs, validate, type Config } from '../config'

describe('github-filter validate', () => {
  it('flags a filter with no groups set — it would pass everything, which is never the intent', () => {
    expect(validate({})).toEqual(['at least one filter group must be set'])
    expect(validate(defaults)).toEqual(['at least one filter group must be set'])
  })

  it('accepts any single non-empty group', () => {
    const groups: Config[] = [
      { repos: ['acme/*'] },
      { exclude_repos: ['acme/*'] },
      { authors: ['octocat'] },
      { exclude_authors: ['*[bot]'] },
      { labels: ['bug'] },
      { exclude_labels: ['wontfix'] },
      { types: ['pr'] },
      { reasons: ['mention'] },
      { ci: ['passing'] },
      { review: ['approved'] },
    ]
    for (const config of groups) {
      expect(validate(config), JSON.stringify(config)).toEqual([])
    }
  })

  it('ignores a group that is present but empty', () => {
    expect(validate({ repos: [] })).toEqual(['at least one filter group must be set'])
  })
})

describe('github-filter ports', () => {
  it('declares two outputs — port 0 passes, port 1 fails', () => {
    expect(outputs).toBe(2)
  })
})
