import { processFile, type FileDiffMetadata, type Hunk } from '@pierre/diffs'
import type {
  ChangedFile,
  SessionFileDiff,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type DiffMode = 'unified' | 'split'

/**
 * Row metrics of the rendered diff. CodeView estimates every off-screen height
 * from these, and DiffStream's CSS must produce the same sizes, or the scrollbar
 * drifts as files mount.
 */
export const DIFF_METRICS = {
  headerPx: 32,
  rowPx: 20,
  separatorPx: 20,
} as const

export function diffHasText(diff: SessionFileDiff | undefined): boolean {
  return diff?.state === 'text' && !!diff.text
}

function changeType(file: ChangedFile): FileDiffMetadata['type'] {
  if (file.status === 'added' || file.status === 'untracked') return 'new'
  if (file.status === 'deleted') return 'deleted'
  return file.oldPath ? 'rename-changed' : 'change'
}

/** A file with nothing to draw yet, or nothing to draw at all: only its header shows. */
export function emptyFileDiff(file: ChangedFile): FileDiffMetadata {
  return {
    name: file.path,
    prevName: file.oldPath || undefined,
    type: changeType(file),
    hunks: [],
    splitLineCount: 0,
    unifiedLineCount: 0,
    isPartial: true,
    deletionLines: [],
    additionLines: [],
    deletionCharacterCount: 0,
    additionCharacterCount: 0,
  }
}

/**
 * Parses one file's unified diff. The parser reads the file name from the header
 * lines, and the backend's text may have none, so it gets fixed ones: a path with
 * spaces or quotes never has to round-trip through a patch header.
 */
export function parseFileDiff(file: ChangedFile, text: string, cacheKey: string): FileDiffMetadata {
  const firstHunk = text.startsWith('@@ ') ? 0 : text.indexOf('\n@@ ') + 1
  if (firstHunk === 0 && !text.startsWith('@@ ')) return emptyFileDiff(file)
  const parsed = processFile(`--- a/file\n+++ b/file\n${text.slice(firstHunk)}`, { cacheKey, isGitDiff: false })
  if (!parsed) return emptyFileDiff(file)
  parsed.name = file.path
  parsed.prevName = file.oldPath || undefined
  parsed.type = changeType(file)
  return parsed
}

/**
 * The top of each hunk, in pixels from the top of its file's header. A patch
 * holds only its hunks, so the rows above one are the hunks before it, plus one
 * separator each; the first hunk has a separator only when it starts mid-file.
 */
export function hunkOffsets(hunks: readonly Hunk[], mode: DiffMode): number[] {
  const { headerPx, rowPx, separatorPx } = DIFF_METRICS
  let top = headerPx
  return hunks.map((hunk, index) => {
    const at = top
    if (index > 0 || hunk.collapsedBefore > 0) top += separatorPx
    top += (mode === 'split' ? hunk.splitLineCount : hunk.unifiedLineCount) * rowPx
    return at
  })
}

/** How the files should be read: outward from the one on screen, so a jump reads what it landed on first. */
export function readOrder<T>(items: readonly T[], center: number): T[] {
  return items
    .map((item, index) => ({ item, distance: Math.abs(index - center) }))
    .sort((a, b) => a.distance - b.distance)
    .map(({ item }) => item)
}
