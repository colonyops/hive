import type { AppletColumn } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { fuzzyMatch } from '../composables/useCommands'

export type AppletRow = Record<string, string>

function rowText(row: AppletRow, columns: readonly AppletColumn[]): string {
  return columns.map((column) => row[column.key] ?? '').join(' ')
}

/** Rows matching a query, best first. An empty query keeps the command's order. */
export function filterAppletRows(
  rows: readonly AppletRow[],
  columns: readonly AppletColumn[],
  query: string,
): AppletRow[] {
  const needle = query.trim()
  if (!needle) return [...rows]
  return rows
    .map((row) => ({ row, match: fuzzyMatch(needle, rowText(row, columns)) }))
    .filter((entry) => entry.match !== null)
    .sort((a, b) => (b.match?.score ?? 0) - (a.match?.score ?? 0))
    .map((entry) => entry.row)
}

/**
 * Where the selection belongs after the rows changed: on the row it was on, read by
 * its first column, because a status changing on refresh must not move it.
 */
export function reselect(
  rows: readonly AppletRow[],
  columns: readonly AppletColumn[],
  previous: AppletRow | undefined,
  index: number,
): number {
  const key = columns[0]?.key
  if (previous && key !== undefined) {
    const found = rows.findIndex((row) => row[key] === previous[key])
    if (found >= 0) return found
  }
  return Math.max(0, Math.min(index, rows.length - 1))
}
