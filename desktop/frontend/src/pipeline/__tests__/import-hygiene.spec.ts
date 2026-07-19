// Import-hygiene guard (D2): a node type's runtime.ts is worker-side code —
// in production it runs inside a Web Worker (WebWorkerTransport) with no
// DOM and no Vue instance. This asserts every nodes/*/runtime.ts stays free
// of both, by reading each file as text rather than importing it (importing
// would silently succeed in vitest's happy-dom environment even for a
// module that reaches for `document`/`window`, since those globals DO exist
// there — text inspection is the only way to catch the mistake).

import { describe, expect, it } from 'vitest'
import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const nodesDir = join(here, '..', 'nodes')

function runtimeFiles(): string[] {
  return readdirSync(nodesDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => join(nodesDir, entry.name, 'runtime.ts'))
    .filter((path) => {
      try {
        readFileSync(path)
        return true
      } catch {
        return false
      }
    })
}

describe('pipeline node runtime import hygiene', () => {
  const files = runtimeFiles()

  it('finds at least one runtime.ts module to check (guards against a silently-empty glob)', () => {
    expect(files.length).toBeGreaterThan(0)
  })

  it('no runtime.ts imports vue', () => {
    for (const file of files) {
      const src = readFileSync(file, 'utf-8')
      expect(src, file).not.toMatch(/from\s+['"]vue['"]/)
      expect(src, file).not.toMatch(/require\(\s*['"]vue['"]\s*\)/)
    }
  })

  it('no runtime.ts reaches for DOM/browser-window globals', () => {
    const domGlobals = [/\bdocument\./, /\bwindow\./, /\blocalStorage\b/, /\bsessionStorage\b/]
    for (const file of files) {
      const src = readFileSync(file, 'utf-8')
      for (const pattern of domGlobals) {
        expect(src, `${file} matched ${pattern}`).not.toMatch(pattern)
      }
    }
  })
})
