import type { ChangedFile } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type GitRow =
  { kind: 'dir'; key: string; label: string; depth: number } | { kind: 'file'; file: ChangedFile; depth: number }

export type GitLayout = 'tree' | 'flat'

interface Folder {
  dirs: Map<string, Folder>
  files: ChangedFile[]
}

// git lists an untracked nested repository as "inner/", so a path can end in a
// slash that names no segment.
function segments(path: string): string[] {
  return path.split('/').filter(Boolean)
}

export function gitFileName(path: string): string {
  return segments(path).at(-1) ?? path
}

export function gitFileDir(path: string): string {
  return segments(path).slice(0, -1).join('/')
}

function buildTree(files: ChangedFile[]): Folder {
  const root: Folder = { dirs: new Map(), files: [] }
  for (const file of files) {
    let folder = root
    for (const part of segments(file.path).slice(0, -1)) {
      let next = folder.dirs.get(part)
      if (!next) {
        next = { dirs: new Map(), files: [] }
        folder.dirs.set(part, next)
      }
      folder = next
    }
    folder.files.push(file)
  }
  return root
}

const byPath = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0)

/**
 * The rows the Git pane draws: folders before files at each level, with a chain
 * of single-child folders on one row so deep paths stay readable in a narrow
 * pane. A folder's key is its full path.
 */
export function gitRows(files: ChangedFile[], layout: GitLayout, isOpen: (key: string) => boolean): GitRow[] {
  if (layout === 'flat') {
    return [...files].sort((a, b) => byPath(a.path, b.path)).map((file) => ({ kind: 'file', file, depth: 0 }))
  }
  const rows: GitRow[] = []
  const walk = (folder: Folder, depth: number, prefix: string) => {
    for (const [name, dir] of [...folder.dirs].sort(([a], [b]) => byPath(a, b))) {
      let current = dir
      let label = name
      while (current.dirs.size === 1 && current.files.length === 0) {
        const [childName, child] = [...current.dirs][0]
        label += `/${childName}`
        current = child
      }
      const key = prefix + label
      rows.push({ kind: 'dir', key, label, depth })
      if (isOpen(key)) walk(current, depth + 1, `${key}/`)
    }
    for (const file of [...folder.files].sort((a, b) => byPath(a.path, b.path))) {
      rows.push({ kind: 'file', file, depth })
    }
  }
  walk(buildTree(files), 0, '')
  return rows
}
