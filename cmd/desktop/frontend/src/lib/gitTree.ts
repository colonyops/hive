import { prepareFileTreeInput } from '@pierre/trees'
import type { ChangedFile } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

// git lists an untracked nested repository as "inner/", so a path can end in a
// slash that names no segment.
function segments(path: string): string[] {
  return path.split('/').filter(Boolean)
}

export function gitFileName(path: string): string {
  return segments(path).at(-1) ?? path
}

const STATUS: Record<string, { letter: string; label: string; color: string }> = {
  modified: { letter: 'M', label: 'Modified', color: 'text-severity-warning' },
  added: { letter: 'A', label: 'Added', color: 'text-severity-success' },
  deleted: { letter: 'D', label: 'Deleted', color: 'text-severity-error' },
  renamed: { letter: 'R', label: 'Renamed', color: 'text-severity-info' },
  untracked: { letter: 'U', label: 'Untracked', color: 'text-severity-success' },
}

export function gitStatus(status: string): { letter: string; label: string; color: string } {
  return STATUS[status] ?? { letter: '?', label: status, color: 'text-text-3' }
}

/** The tree path of a changed file: git names an untracked nested repository "inner/", which the tree would read as a folder. */
export function gitTreePath(path: string): string {
  return segments(path).join('/')
}

/**
 * The changed files in the order the tree lists them (folders first, natural
 * name order), so j and k in the diff stream walk the rail row by row.
 */
export function gitOrder(files: readonly ChangedFile[]): ChangedFile[] {
  const byTreePath = new Map(files.map((file) => [gitTreePath(file.path), file]))
  return prepareFileTreeInput([...byTreePath.keys()]).paths.flatMap((path) => byTreePath.get(path) ?? [])
}
