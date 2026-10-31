import type { SelectedLineRange } from '@pierre/diffs'

export type DiffSide = 'additions' | 'deletions'

export interface DiffRow {
  type: 'context' | 'addition' | 'deletion'
  /** The row's line in the old version; 0 on an added row. */
  oldLine: number
  /** The row's line in the new version; 0 on a removed row. */
  newLine: number
  text: string
}

/** Parses one file's patch text into its rows, in the order a unified diff shows them. */
export function diffRows(patch: string): DiffRow[] {
  const rows: DiffRow[] = []
  let oldLine = 0
  let newLine = 0
  let inHunk = false
  for (const line of patch.split('\n')) {
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line)
    if (hunk) {
      oldLine = Number(hunk[1])
      newLine = Number(hunk[2])
      inHunk = true
    } else if (!inHunk) {
      continue
    } else if (line.startsWith('+')) {
      rows.push({ type: 'addition', oldLine: 0, newLine: newLine++, text: line.slice(1) })
    } else if (line.startsWith('-')) {
      rows.push({ type: 'deletion', oldLine: oldLine++, newLine: 0, text: line.slice(1) })
    } else if (line.startsWith(' ')) {
      rows.push({ type: 'context', oldLine: oldLine++, newLine: newLine++, text: line.slice(1) })
    }
  }
  return rows
}

function rowIndex(rows: readonly DiffRow[], line: number, side: DiffSide): number {
  return rows.findIndex((row) => (side === 'deletions' ? row.oldLine : row.newLine) === line)
}

export function hasDiffLine(rows: readonly DiffRow[], line: number, side: DiffSide): boolean {
  return rowIndex(rows, line, side) >= 0
}

const rowSide = (row: DiffRow): DiffSide => (row.type === 'deletion' ? 'deletions' : 'additions')
const rowLine = (row: DiffRow): number => (row.type === 'deletion' ? row.oldLine : row.newLine)
const ROW_PREFIX: Record<DiffRow['type'], string> = { context: ' ', addition: '+', deletion: '-' }

export interface DiffSpan {
  startSide: DiffSide
  startLine: number
  endSide: DiffSide
  endLine: number
  /** The rows as patch text, so feedback reads the same whatever happens to the file later. */
  quote: string
}

/**
 * The rows a selection covers, first to last. A drag upward arrives reversed, and
 * numbers alone cannot put it right: a removed row can carry a higher number than
 * the added row below it, so the order comes from the rows.
 */
export function diffSpan(rows: readonly DiffRow[], range: SelectedLineRange): DiffSpan | null {
  const side = range.side ?? 'additions'
  const from = rowIndex(rows, range.start, side)
  const to = rowIndex(rows, range.end, range.endSide ?? side)
  if (from < 0 || to < 0) return null
  const covered = rows.slice(Math.min(from, to), Math.max(from, to) + 1)
  const first = covered[0]
  const last = covered[covered.length - 1]
  return {
    startSide: rowSide(first),
    startLine: rowLine(first),
    endSide: rowSide(last),
    endLine: rowLine(last),
    quote: covered.map((row) => ROW_PREFIX[row.type] + row.text).join('\n'),
  }
}

export interface DiffSearchMatch {
  path: string
  side: DiffSide
  line: number
  /** Which hit on its line this is, counting from 0. */
  occurrence: number
}

/** Past this many a search stops counting: nobody steps through them, and a common word could reach millions. */
export const MAX_SEARCH_MATCHES = 2000

export function occurrencesIn(text: string, needle: string): number[] {
  const hits: number[] = []
  const haystack = text.toLowerCase()
  for (let at = haystack.indexOf(needle); at !== -1; at = haystack.indexOf(needle, at + needle.length)) hits.push(at)
  return hits
}

/** Case-insensitive matches over files in the order given. */
export function searchDiffRows(
  files: readonly { path: string; rows: readonly DiffRow[] }[],
  query: string,
): DiffSearchMatch[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return []
  const matches: DiffSearchMatch[] = []
  for (const { path, rows } of files) {
    for (const row of rows) {
      const hits = occurrencesIn(row.text, needle).length
      for (let occurrence = 0; occurrence < hits; occurrence++) {
        matches.push({ path, side: rowSide(row), line: rowLine(row), occurrence })
        if (matches.length >= MAX_SEARCH_MATCHES) return matches
      }
    }
  }
  return matches
}

export const sameMatch = (a: DiffSearchMatch, b: DiffSearchMatch): boolean =>
  a.path === b.path && a.side === b.side && a.line === b.line && a.occurrence === b.occurrence

/** The label a comment's lines show: `L12`, `L12-14`, and `old L7` for a removed line. */
export function spanLabel(span: Pick<DiffSpan, 'startSide' | 'startLine' | 'endSide' | 'endLine'>): string {
  const old = (side: DiffSide) => (side === 'deletions' ? 'old ' : '')
  if (span.startSide !== span.endSide) {
    return `${old(span.startSide)}L${span.startLine} to ${old(span.endSide)}L${span.endLine}`
  }
  const lines = span.startLine === span.endLine ? `${span.startLine}` : `${span.startLine}-${span.endLine}`
  return `${old(span.startSide)}L${lines}`
}
