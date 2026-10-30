import { describe, expect, it } from 'vitest'
import { diffLanguage, highlightedLines, parseUnifiedDiff, splitRows } from '../unifiedDiff'

const PATCH = `diff --git a/a.go b/a.go
index 111..222 100644
--- a/a.go
+++ b/a.go
@@ -10,3 +10,4 @@ func a() {
 keep
-old
+new
+more
@@ -40 +41 @@
-last
+last
\\ No newline at end of file
`

describe('parseUnifiedDiff', () => {
  it('numbers each side and skips the file headers', () => {
    const hunks = parseUnifiedDiff(PATCH)
    expect(hunks.map((hunk) => hunk.header)).toEqual(['@@ -10,3 +10,4 @@ func a() {', '@@ -40 +41 @@'])
    expect(hunks[0].lines).toEqual([
      { kind: 'context', text: 'keep', oldNumber: 10, newNumber: 10 },
      { kind: 'del', text: 'old', oldNumber: 11 },
      { kind: 'add', text: 'new', newNumber: 11 },
      { kind: 'add', text: 'more', newNumber: 12 },
    ])
  })

  it('drops the no-newline marker without moving the numbers', () => {
    const [, second] = parseUnifiedDiff(PATCH)
    expect(second.lines).toEqual([
      { kind: 'del', text: 'last', oldNumber: 40 },
      { kind: 'add', text: 'last', newNumber: 41 },
    ])
  })

  it('keeps a line whose content looks like a header', () => {
    const [hunk] = parseUnifiedDiff('@@ -1 +1,2 @@\n+--- a/x\n+@@ not a hunk\n')
    expect(hunk.lines.map((line) => line.text)).toEqual(['--- a/x', '@@ not a hunk'])
  })

  it('reads a blank line in a hunk as blank context, with diff.suppressBlankEmpty', () => {
    const [hunk] = parseUnifiedDiff('@@ -1,2 +1,2 @@\n\n+x\n')
    expect(hunk.lines[0]).toEqual({ kind: 'context', text: '', oldNumber: 1, newNumber: 1 })
  })

  it('has no hunks for a rename with no edits or an empty diff', () => {
    expect(parseUnifiedDiff('similarity index 100%\nrename from a\nrename to b\n')).toEqual([])
    expect(parseUnifiedDiff('')).toEqual([])
  })
})

describe('splitRows', () => {
  it('pairs a run of deletions with the additions after it', () => {
    const [hunk] = parseUnifiedDiff('@@ -1,3 +1,3 @@\n c\n-a\n-b\n+x\n d\n')
    const rows = splitRows(hunk.lines)
    expect(rows.map((row) => [row.left?.text, row.right?.text])).toEqual([
      ['c', 'c'],
      ['a', 'x'],
      ['b', undefined],
      ['d', 'd'],
    ])
  })
})

describe('diffLanguage', () => {
  it('reads the extension, an alias, or a Dockerfile', () => {
    expect(diffLanguage('cmd/a.GO')).toBe('go')
    expect(diffLanguage('lib/x.mjs')).toBe('js')
    expect(diffLanguage('deploy/Dockerfile')).toBe('dockerfile')
    expect(diffLanguage('Makefile')).toBe('')
  })
})

describe('highlightedLines', () => {
  it('splits the code block into one entry per line', () => {
    const html = '<pre class="shiki"><code><span class="line">a</span>\n<span class="line">b</span></code></pre>'
    expect(highlightedLines(html)).toEqual(['<span class="line">a</span>', '<span class="line">b</span>'])
  })
})
