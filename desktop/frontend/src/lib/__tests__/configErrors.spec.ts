import { describe, expect, it } from 'vitest'
import { parseConfigErrors } from '../configErrors'

describe('parseConfigErrors', () => {
  it('returns no entries for an empty or blank string', () => {
    expect(parseConfigErrors('')).toEqual([])
    expect(parseConfigErrors('   \n  ')).toEqual([])
  })

  it('treats a plain fmt.Errorf-style message as one unnumbered entry', () => {
    expect(parseConfigErrors('source "x": kind "search" requires a query')).toEqual([
      { line: null, message: 'source "x": kind "search" requires a query' },
    ])
  })

  it('splits multi-line yaml.v3 "line N:" errors into separate numbered entries', () => {
    const raw = 'yaml: unmarshal errors:\n  line 4: field foo not found in type feed.configFile\n  line 9: field bar not found in type feed.configFile'
    expect(parseConfigErrors(raw)).toEqual([
      { line: 4, message: 'field foo not found in type feed.configFile' },
      { line: 9, message: 'field bar not found in type feed.configFile' },
    ])
  })

  it('parses a single "line N:" prefixed message', () => {
    expect(parseConfigErrors('yaml: line 3: mapping values are not allowed in this context')).toEqual([
      { line: 3, message: 'mapping values are not allowed in this context' },
    ])
  })
})
