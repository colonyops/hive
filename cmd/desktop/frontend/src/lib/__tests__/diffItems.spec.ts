import { describe, expect, it } from 'vitest'
import { DIFF_METRICS, diffHasText, emptyFileDiff, hunkOffsets, parseFileDiff, readOrder } from '../diffItems'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const file = (extra: Partial<ChangedFile> = {}): ChangedFile => ({
  path: 'dir/a file.go',
  oldPath: '',
  status: 'modified',
  additions: 1,
  deletions: 1,
  binary: false,
  stamp: '1',
  ...extra,
})
const GIT_HEADER =
  'diff --git a/dir/a file.go b/dir/a file.go\nindex 1..2 100644\n--- a/dir/a file.go\n+++ b/dir/a file.go\n'
const TWO_HUNKS = '@@ -10,2 +10,3 @@ func x() {\n ctx\n-old\n+new\n+more\n@@ -40 +41 @@\n-var a\n+var b\n'

describe('parseFileDiff', () => {
  it('keeps the real path and reads git headers and bare hunks the same', () => {
    const bare = parseFileDiff(file(), TWO_HUNKS, 'k')
    const withHeader = parseFileDiff(file(), GIT_HEADER + TWO_HUNKS, 'k')
    expect(bare.name).toBe('dir/a file.go')
    expect(bare.type).toBe('change')
    expect(bare.cacheKey).toBe('k')
    expect(bare.hunks).toHaveLength(2)
    expect(withHeader.hunks.map((hunk) => hunk.additionStart)).toEqual(bare.hunks.map((hunk) => hunk.additionStart))
  })

  it('takes the change type from the file, not the patch', () => {
    expect(parseFileDiff(file({ status: 'untracked' }), '@@ -0,0 +1 @@\n+x\n', 'k').type).toBe('new')
    expect(parseFileDiff(file({ status: 'deleted' }), '@@ -1 +0,0 @@\n-x\n', 'k').type).toBe('deleted')
    const renamed = parseFileDiff(file({ oldPath: 'old.go' }), TWO_HUNKS, 'k')
    expect(renamed.type).toBe('rename-changed')
    expect(renamed.prevName).toBe('old.go')
  })

  it('answers an empty diff for text with no hunk', () => {
    expect(parseFileDiff(file(), GIT_HEADER, 'k').hunks).toEqual([])
    expect(emptyFileDiff(file()).name).toBe('dir/a file.go')
  })
})

describe('diffHasText', () => {
  const diff = { path: 'a', state: 'text', text: '@@', error: '' }
  it('is true only for a diff with text and no other state', () => {
    expect(diffHasText(diff)).toBe(true)
    expect(diffHasText(undefined)).toBe(false)
    expect(diffHasText({ ...diff, text: '' })).toBe(false)
    expect(diffHasText({ ...diff, state: 'binary' })).toBe(false)
    expect(diffHasText({ ...diff, state: 'error' })).toBe(false)
  })
})

describe('hunkOffsets', () => {
  const { headerPx, rowPx, separatorPx } = DIFF_METRICS
  it('counts rendered rows and separators, not the file lines the patch leaves out', () => {
    const { hunks } = parseFileDiff(file(), TWO_HUNKS, 'k')
    const unifiedRows = hunks[0].unifiedLineCount
    const splitRows = hunks[0].splitLineCount
    expect(hunkOffsets(hunks, 'unified')).toEqual([headerPx, headerPx + separatorPx + unifiedRows * rowPx])
    expect(hunkOffsets(hunks, 'split')).toEqual([headerPx, headerPx + separatorPx + splitRows * rowPx])
  })

  it('gives a first hunk at the top of the file no separator', () => {
    const { hunks } = parseFileDiff(file(), '@@ -1,2 +1,2 @@\n-a\n+b\n c\n@@ -40 +40 @@\n-d\n+e\n', 'k')
    expect(hunkOffsets(hunks, 'unified')).toEqual([headerPx, headerPx + 3 * rowPx])
  })
})

describe('readOrder', () => {
  it('walks outward from the center', () => {
    expect(readOrder(['a', 'b', 'c', 'd', 'e'], 2)).toEqual(['c', 'b', 'd', 'a', 'e'])
    expect(readOrder(['a', 'b'], -1)).toEqual(['a', 'b'])
  })
})
