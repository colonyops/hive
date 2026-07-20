// Cross-registry parity guard: the app registry (byType, over
// nodes/*/index.ts) and the worker registry (processorRegistry,
// over nodes/*/runtime.ts) are discovered independently via two separate
// import.meta.glob calls — nothing structurally forces them to agree.  This
// asserts they do: every `role: 'processor'` app entry has a matching
// worker runtime, and every worker runtime has a matching app entry with
// role 'processor'. source/output-role entries are asserted to have NO
// runtime (github-source runs in Go; feed/action are engine-tagged
// terminals) — the opposite drift (an accidental runtime.ts for a
// non-processor type) would silently ship dead worker code.

import { describe, expect, it } from 'vitest'
import { byType } from '../registry'
import { processorRegistry } from '../processors'

describe('app/worker registry parity', () => {
  it('every processor-role app entry has a matching worker runtime', () => {
    const processors = Object.values(byType).filter((def) => def.role === 'processor')
    expect(processors.length).toBeGreaterThan(0)
    for (const def of processors) {
      expect(processorRegistry[def.type], `no runtime.ts registered for processor "${def.type}"`).toBeDefined()
    }
  })

  it('every worker runtime has a matching processor-role app entry', () => {
    for (const type of Object.keys(processorRegistry)) {
      const def = byType[type]
      expect(def, `no app registry entry for runtime "${type}"`).toBeDefined()
      expect(def!.role).toBe('processor')
    }
  })

  it('non-processor app entries (source/output) have no worker runtime', () => {
    const nonProcessors = Object.values(byType).filter((def) => def.role !== 'processor')
    expect(nonProcessors.length).toBeGreaterThan(0)
    for (const def of nonProcessors) {
      expect(processorRegistry[def.type]).toBeUndefined()
    }
  })
})
