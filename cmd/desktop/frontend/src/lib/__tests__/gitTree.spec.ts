import { describe, expect, it } from 'vitest'
import { gitFileName, gitOrder, gitTreePath } from '../gitTree'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

function file(path: string): ChangedFile {
  return { path, oldPath: '', status: 'modified', additions: 0, deletions: 0, binary: false, stamp: '' }
}

describe('gitOrder', () => {
  it('lists folders before files at each level, as the tree does', () => {
    const order = gitOrder([file('a.ts'), file('z/b.ts'), file('z/y/c.ts')]).map((f) => f.path)
    expect(order).toEqual(['z/y/c.ts', 'z/b.ts', 'a.ts'])
  })

  // git lists an untracked nested repository as "inner/".
  it('keeps a trailing-slash path as a file of the tree', () => {
    expect(gitTreePath('pkg/inner/')).toBe('pkg/inner')
    expect(gitOrder([file('pkg/inner/'), file('pkg/a.ts')]).map((f) => f.path)).toEqual(['pkg/a.ts', 'pkg/inner/'])
    expect(gitFileName('pkg/inner/')).toBe('inner')
  })
})
