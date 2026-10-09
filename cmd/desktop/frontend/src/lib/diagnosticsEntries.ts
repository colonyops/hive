import type { Entry as DiagnosticEntry } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/diagnostics/models'

function fieldsEqual(left: DiagnosticEntry['fields'], right: DiagnosticEntry['fields']): boolean {
  if (left === right) return true
  if (!left || !right) return false
  const keys = Object.keys(left)
  if (keys.length !== Object.keys(right).length) return false
  return keys.every((key) => left[key] === right[key])
}

function entriesEqual(left: DiagnosticEntry, right: DiagnosticEntry): boolean {
  return (
    left.id === right.id &&
    left.time === right.time &&
    left.source === right.source &&
    left.level === right.level &&
    left.message === right.message &&
    left.raw === right.raw &&
    left.truncated === right.truncated &&
    fieldsEqual(left.fields, right.fields)
  )
}

export function retainUnchangedDiagnosticEntries(
  previous: DiagnosticEntry[],
  incoming: DiagnosticEntry[],
): DiagnosticEntry[] {
  const previousByKey = new Map(previous.map((entry) => [`${entry.id}\u0000${entry.time}`, entry]))
  const retained = incoming.map((entry) => {
    const existing = previousByKey.get(`${entry.id}\u0000${entry.time}`)
    return existing && entriesEqual(existing, entry) ? existing : entry
  })

  if (retained.length === previous.length && retained.every((entry, index) => entry === previous[index])) {
    return previous
  }
  return retained
}
