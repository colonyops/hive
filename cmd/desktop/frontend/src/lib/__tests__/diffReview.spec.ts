import { describe, expect, it } from 'vitest'
import { MAX_SEARCH_MATCHES, diffRows, diffSpan, hasDiffLine, searchDiffRows, spanLabel } from '../diffReview'

const PATCH = [
  'diff --git a/a.go b/a.go',
  '--- a/a.go',
  '+++ b/a.go',
  '@@ -10,3 +10,4 @@ func read() {',
  ' keep',
  '-old Value',
  '+new value',
  '+another VALUE value',
  ' tail',
  '\\ No newline at end of file',
  '',
].join('\n')

describe('diffRows', () => {
  it('numbers each row on its own side and skips everything outside the rows', () => {
    expect(diffRows(PATCH)).toEqual([
      { type: 'context', oldLine: 10, newLine: 10, text: 'keep' },
      { type: 'deletion', oldLine: 11, newLine: 0, text: 'old Value' },
      { type: 'addition', oldLine: 0, newLine: 11, text: 'new value' },
      { type: 'addition', oldLine: 0, newLine: 12, text: 'another VALUE value' },
      { type: 'context', oldLine: 12, newLine: 13, text: 'tail' },
    ])
  })
})

describe('diffSpan', () => {
  const rows = diffRows(PATCH)

  it('quotes the rows as patch text', () => {
    expect(diffSpan(rows, { start: 10, side: 'additions', end: 11, endSide: 'additions' })?.quote).toBe(
      ' keep\n-old Value\n+new value',
    )
  })

  it('puts a drag upward back in row order, even across sides', () => {
    expect(diffSpan(rows, { start: 11, side: 'additions', end: 11, endSide: 'deletions' })).toMatchObject({
      startSide: 'deletions',
      startLine: 11,
      endSide: 'additions',
      endLine: 11,
    })
  })

  it('finds a context row from either side', () => {
    expect(diffSpan(rows, { start: 12, side: 'deletions', end: 12, endSide: 'deletions' })).toMatchObject({
      startSide: 'additions',
      startLine: 13,
    })
  })

  it('is null for a line the diff does not hold', () => {
    expect(diffSpan(rows, { start: 99, side: 'additions', end: 99 })).toBeNull()
    expect(hasDiffLine(rows, 99, 'additions')).toBe(false)
    expect(hasDiffLine(rows, 11, 'deletions')).toBe(true)
  })
})

describe('searchDiffRows', () => {
  const files = [{ path: 'a.go', rows: diffRows(PATCH) }]

  it('ignores case and counts every hit on a line', () => {
    expect(searchDiffRows(files, ' value ')).toEqual([
      { path: 'a.go', side: 'deletions', line: 11, occurrence: 0 },
      { path: 'a.go', side: 'additions', line: 11, occurrence: 0 },
      { path: 'a.go', side: 'additions', line: 12, occurrence: 0 },
      { path: 'a.go', side: 'additions', line: 12, occurrence: 1 },
    ])
  })

  it('finds nothing for an empty query', () => {
    expect(searchDiffRows(files, '  ')).toEqual([])
  })

  it('stops counting at the cap', () => {
    const rows = Array.from({ length: MAX_SEARCH_MATCHES + 5 }, (_, i) => ({
      type: 'addition' as const,
      oldLine: 0,
      newLine: i + 1,
      text: 'x',
    }))
    expect(searchDiffRows([{ path: 'a', rows }], 'x')).toHaveLength(MAX_SEARCH_MATCHES)
  })
})

describe('spanLabel', () => {
  it.each([
    [{ startSide: 'additions', startLine: 4, endSide: 'additions', endLine: 4 }, 'L4'],
    [{ startSide: 'additions', startLine: 4, endSide: 'additions', endLine: 9 }, 'L4-9'],
    [{ startSide: 'deletions', startLine: 4, endSide: 'deletions', endLine: 9 }, 'old L4-9'],
    [{ startSide: 'deletions', startLine: 4, endSide: 'additions', endLine: 9 }, 'old L4 to L9'],
  ] as const)('labels %j as %s', (span, label) => {
    expect(spanLabel(span)).toBe(label)
  })
})
