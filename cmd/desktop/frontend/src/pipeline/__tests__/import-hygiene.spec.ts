// Execution-stays-in-Go guard.
//
// This used to police the boundary between a node's editor module and its
// worker-side runtime.ts. There is no worker-side runtime any more: the flow
// engine is `internal/app/runtime`, and a node type's execution is a Go
// registry line (ADR flow-engine-in-go). What the frontend still owns is the editor —
// config.ts, editor.vue, index.ts.
//
// The failure this guards against is a quiet one. Adding a runtime.ts back
// would not break anything visibly: it would ship a second implementation of
// a node's semantics that nothing calls, and the next person to change the
// rule would change only one of the two. Files are read as text rather than
// imported, so a module is caught whether or not anything references it.

import { describe, expect, it } from 'vitest'
import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const nodesDir = join(here, '..', 'nodes')

function nodeDirs(): string[] {
  return readdirSync(nodesDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
}

function readIfPresent(path: string): string | null {
  try {
    return readFileSync(path, 'utf-8')
  } catch {
    return null
  }
}

describe('node execution stays in Go', () => {
  const dirs = nodeDirs()

  it('finds the node type directories to check (guards against a silently-empty scan)', () => {
    expect(dirs.length).toBeGreaterThan(0)
  })

  it('no node type has a runtime.ts — the engine that runs nodes is internal/app/runtime', () => {
    for (const dir of dirs) {
      expect(readIfPresent(join(nodesDir, dir, 'runtime.ts')), `nodes/${dir}/runtime.ts must not exist`).toBeNull()
    }
  })

  it('no editor module imports a runtime module', () => {
    for (const dir of dirs) {
      for (const file of ['index.ts', 'config.ts', 'editor.vue']) {
        const src = readIfPresent(join(nodesDir, dir, file))
        if (src === null) continue
        expect(src, `nodes/${dir}/${file}`).not.toMatch(/from\s+['"]\.\/runtime['"]/)
        expect(src, `nodes/${dir}/${file}`).not.toMatch(/require\(\s*['"]\.\/runtime['"]\s*\)/)
      }
    }
  })
})
