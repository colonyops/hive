export interface DiffLine {
  kind: 'context' | 'add' | 'del'
  text: string
  oldNumber?: number
  newNumber?: number
}

export interface DiffHunk {
  header: string
  lines: DiffLine[]
}

export interface SplitRow {
  left?: DiffLine
  right?: DiffLine
}

const HUNK_HEADER = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/

/** Reads the hunks of one file's unified diff. The headers before the first hunk carry nothing to show. */
export function parseUnifiedDiff(text: string): DiffHunk[] {
  const hunks: DiffHunk[] = []
  let hunk: DiffHunk | null = null
  let oldNumber = 0
  let newNumber = 0
  const lines = text.split('\n')
  if (lines.at(-1) === '') lines.pop()
  for (const line of lines) {
    const header = HUNK_HEADER.exec(line)
    if (header) {
      hunk = { header: line, lines: [] }
      hunks.push(hunk)
      oldNumber = Number(header[1])
      newNumber = Number(header[2])
      continue
    }
    if (!hunk) continue
    const body = line.slice(1)
    if (line.startsWith('+')) hunk.lines.push({ kind: 'add', text: body, newNumber: newNumber++ })
    else if (line.startsWith('-')) hunk.lines.push({ kind: 'del', text: body, oldNumber: oldNumber++ })
    else if (line.startsWith('\\')) continue
    // diff.suppressBlankEmpty writes a blank context line with no leading space.
    else hunk.lines.push({ kind: 'context', text: body, oldNumber: oldNumber++, newNumber: newNumber++ })
  }
  return hunks
}

/** Pairs a run of deletions with the additions after it, so an edited line sits beside its old self. */
export function splitRows(lines: DiffLine[]): SplitRow[] {
  const rows: SplitRow[] = []
  let dels: DiffLine[] = []
  let adds: DiffLine[] = []
  const flush = () => {
    for (let i = 0; i < Math.max(dels.length, adds.length); i++) rows.push({ left: dels[i], right: adds[i] })
    dels = []
    adds = []
  }
  for (const line of lines) {
    if (line.kind === 'del') {
      if (adds.length) flush()
      dels.push(line)
    } else if (line.kind === 'add') adds.push(line)
    else {
      flush()
      rows.push({ left: line, right: line })
    }
  }
  flush()
  return rows
}

const LANGUAGE_BY_EXTENSION: Record<string, string> = { mjs: 'js', cjs: 'js', mts: 'ts', cts: 'ts' }

/** The highlighter language for a path. The highlighter leaves one it doesn't know as plain text. */
export function diffLanguage(path: string): string {
  const name = path.split('/').pop() ?? path
  if (name === 'Dockerfile') return 'dockerfile'
  const dot = name.lastIndexOf('.')
  if (dot < 0) return ''
  const extension = name.slice(dot + 1).toLowerCase()
  return LANGUAGE_BY_EXTENSION[extension] ?? extension
}

/**
 * The per-line HTML of shiki's output. Shiki joins its `.line` spans with a
 * newline, and a token never holds a raw one, so a split on it is exact.
 */
export function highlightedLines(html: string): string[] {
  const open = html.indexOf('<code>')
  const close = html.lastIndexOf('</code>')
  if (open < 0 || close < 0) return []
  return html.slice(open + '<code>'.length, close).split('\n')
}
