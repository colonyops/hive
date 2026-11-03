import { hydratePartialDiff } from '@pierre/diffs'
import { describe, expect, it } from 'vitest'
import { hunksMatch, wholeFileFromPatch } from '../diffContext'
import { parseFileDiff } from '../diffItems'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const file: ChangedFile = {
  path: 'a.txt',
  oldPath: '',
  status: 'modified',
  additions: 2,
  deletions: 2,
  binary: false,
  stamp: '1',
}

const OLD = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`)
const NEW = OLD.map((line, i) => (i === 4 ? 'five' : i === 15 ? 'sixteen' : line))

function wholePatch(): string {
  const rows = OLD.flatMap((line, i) => (line === NEW[i] ? [` ${line}`] : [`-${line}`, `+${NEW[i]}`]))
  return `diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1,20 +1,20 @@\n${rows.join('\n')}\n`
}

const PARTIAL =
  '@@ -2,7 +2,7 @@\n line 2\n line 3\n line 4\n-line 5\n+five\n line 6\n line 7\n line 8\n' +
  '@@ -13,7 +13,7 @@\n line 13\n line 14\n line 15\n-line 16\n+sixteen\n line 17\n line 18\n line 19\n'

describe('wholeFileFromPatch', () => {
  it('rebuilds both versions', () => {
    expect(wholeFileFromPatch(wholePatch())).toEqual({ old: OLD.join('\n') + '\n', new: NEW.join('\n') + '\n' })
  })

  it('keeps a missing final newline to the side that lacks it', () => {
    const patch = '@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n'
    expect(wholeFileFromPatch(patch)).toEqual({ old: 'a\nb', new: 'a\nb\n' })
    expect(wholeFileFromPatch('@@ -1 +1 @@\n a\n\\ No newline at end of file\n')).toEqual({ old: 'a', new: 'a' })
  })

  it('refuses a patch that leaves lines out', () => {
    expect(wholeFileFromPatch(PARTIAL)).toBeNull()
  })

  it('gives an empty side for an added file', () => {
    expect(wholeFileFromPatch('@@ -0,0 +1 @@\n+x\n')).toEqual({ old: '', new: 'x\n' })
  })
})

describe('hunksMatch', () => {
  it('accepts the whole file the hunks came from, and hydrates with it', () => {
    const fileDiff = parseFileDiff(file, PARTIAL, 'k')
    const whole = wholeFileFromPatch(wholePatch())!
    expect(hunksMatch(fileDiff, whole)).toBe(true)
    hydratePartialDiff('merge', fileDiff, {
      oldFile: { name: 'a.txt', contents: whole.old },
      newFile: { name: 'a.txt', contents: whole.new },
    })
    expect(fileDiff.isPartial).toBe(false)
    expect(fileDiff.additionLines).toHaveLength(20)
  })

  it('refuses a file that changed since the hunks were read', () => {
    const fileDiff = parseFileDiff(file, PARTIAL, 'k')
    const whole = wholeFileFromPatch(wholePatch())!
    expect(hunksMatch(fileDiff, { ...whole, new: whole.new.replace('sixteen', 'SIXTEEN') })).toBe(false)
  })
})
