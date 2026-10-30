import { describe, expect, it } from 'vitest'
import { gitFileDir, gitFileName, gitRows, type GitRow } from '../gitTree'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

function file(path: string): ChangedFile {
  return { path, oldPath: '', status: 'modified', additions: 0, deletions: 0, binary: false, stamp: '' }
}

const describeRow = (row: GitRow) =>
  `${' '.repeat(row.depth)}${row.kind === 'dir' ? `${row.label}/` : gitFileName(row.file.path)}`

describe('gitRows', () => {
  const open = () => true

  it('compacts a chain of single-child folders onto one row', () => {
    const rows = gitRows([file('cmd/desktop/frontend/a.ts'), file('cmd/desktop/frontend/b.ts')], 'tree', open)
    expect(rows.map(describeRow)).toEqual(['cmd/desktop/frontend/', ' a.ts', ' b.ts'])
  })

  it('stops compacting at a folder that holds a file or two folders', () => {
    const rows = gitRows([file('a/x.ts'), file('a/b/y.ts'), file('c/d/z.ts'), file('c/e/z.ts')], 'tree', open)
    expect(rows.map(describeRow)).toEqual(['a/', ' b/', '  y.ts', ' x.ts', 'c/', ' d/', '  z.ts', ' e/', '  z.ts'])
  })

  it('lists folders before files', () => {
    const rows = gitRows([file('a.ts'), file('z/b.ts')], 'tree', open)
    expect(rows.map(describeRow)).toEqual(['z/', ' b.ts', 'a.ts'])
  })

  it('hides what a folded folder holds, keyed by its full path', () => {
    const rows = gitRows([file('a/b/x.ts'), file('a/c/y.ts')], 'tree', (key) => key !== 'a/b')
    expect(rows.map(describeRow)).toEqual(['a/', ' b/', ' c/', '  y.ts'])
  })

  it('flattens to sorted files in the flat layout', () => {
    const rows = gitRows([file('z.ts'), file('a/b.ts')], 'flat', open)
    expect(rows.map((row) => (row.kind === 'file' ? row.file.path : ''))).toEqual(['a/b.ts', 'z.ts'])
    expect(rows.every((row) => row.depth === 0)).toBe(true)
  })

  // git lists an untracked nested repository as "inner/".
  it('reads a trailing slash as the last segment, not as an empty name', () => {
    const rows = gitRows([file('pkg/inner/')], 'tree', open)
    expect(rows.map(describeRow)).toEqual(['pkg/', ' inner'])
    expect(gitFileName('pkg/inner/')).toBe('inner')
    expect(gitFileDir('pkg/inner/')).toBe('pkg')
  })
})
