import type { FileDiffMetadata } from '@pierre/diffs'

export interface WholeFile {
  old: string
  new: string
}

/**
 * Rebuilds both versions of a file from its diff with the whole file as context.
 * Returns null for text that is not one: a patch with more than one hunk left
 * lines out.
 */
export function wholeFileFromPatch(patch: string): WholeFile | null {
  const oldLines: string[] = []
  const newLines: string[] = []
  let oldEndsInNewline = true
  let newEndsInNewline = true
  let hunks = 0
  let last = ''
  for (const line of patch.split('\n')) {
    if (line.startsWith('@@ ')) {
      if (++hunks > 1) return null
      continue
    }
    if (!hunks) continue
    const body = line.slice(1)
    switch (line[0]) {
      case ' ':
        oldLines.push(body)
        newLines.push(body)
        break
      case '-':
        oldLines.push(body)
        break
      case '+':
        newLines.push(body)
        break
      case '\\':
        if (last !== '+') oldEndsInNewline = false
        if (last !== '-') newEndsInNewline = false
        break
      default:
        continue
    }
    last = line[0]
  }
  const join = (lines: string[], newline: boolean) => (lines.length ? lines.join('\n') + (newline ? '\n' : '') : '')
  return { old: join(oldLines, oldEndsInNewline), new: join(newLines, newEndsInNewline) }
}

const bare = (line: string | undefined) => line?.replace(/\n$/, '')

/**
 * Whether the hunks on screen still sit where the whole file puts them. The file
 * can change between the diff on screen and the read of the whole file, and lines
 * from another version would land between the hunks of this one.
 */
export function hunksMatch(fileDiff: FileDiffMetadata, whole: WholeFile): boolean {
  const oldLines = whole.old.split('\n')
  const newLines = whole.new.split('\n')
  return fileDiff.hunks.every((hunk) => {
    for (let i = 0; i < hunk.additionCount; i++) {
      if (bare(fileDiff.additionLines[hunk.additionLineIndex + i]) !== newLines[hunk.additionStart - 1 + i])
        return false
    }
    for (let i = 0; i < hunk.deletionCount; i++) {
      if (bare(fileDiff.deletionLines[hunk.deletionLineIndex + i]) !== oldLines[hunk.deletionStart - 1 + i])
        return false
    }
    return true
  })
}
