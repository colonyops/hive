import { describe, expect, it } from 'vitest'
import { canConnect } from '../ports'
import type { NodeTypeDefinition } from '../../nodeType'
import type { FlowNode, Wire } from '../../types'

function def(overrides: Partial<NodeTypeDefinition> = {}): NodeTypeDefinition {
  return {
    type: 'fake',
    label: 'Fake',
    category: 'Process',
    role: 'processor',
    glyph: null,
    defaults: {},
    editor: null,
    help: '',
    ...overrides,
  }
}

function node(overrides: Partial<FlowNode> = {}): FlowNode {
  return { id: 'n', type: 'fake', config: {}, ...overrides }
}

// A tiny fake registry — canConnect takes defForType injected rather than
// importing the real byType registry, so a handful of hand-built defs is
// enough to exercise every rule without pulling in every node type.
const defs: Record<string, NodeTypeDefinition> = {
  source: def({ type: 'source', role: 'source', outputs: 1 }),
  processor: def({ type: 'processor', role: 'processor', outputs: 1 }),
  'two-out': def({ type: 'two-out', role: 'processor', outputs: 2 }),
  output: def({ type: 'output', role: 'output' }),
}
const defForType = (type: string) => defs[type]

describe('canConnect', () => {
  it('allows a processor output to wire into another node\'s input', () => {
    const from = node({ id: 'a', type: 'processor' })
    const to = node({ id: 'b', type: 'processor' })
    expect(canConnect(from, 0, to, [], defForType)).toBe(true)
  })

  it('allows wiring into an output (sink) node — it has an input, just no outputs', () => {
    const from = node({ id: 'a', type: 'processor' })
    const to = node({ id: 'b', type: 'output' })
    expect(canConnect(from, 0, to, [], defForType)).toBe(true)
  })

  it('rejects a self-connection', () => {
    const a = node({ id: 'a', type: 'processor' })
    expect(canConnect(a, 0, a, [], defForType)).toBe(false)
  })

  it('rejects a target with no input port (a source node)', () => {
    const from = node({ id: 'a', type: 'processor' })
    const to = node({ id: 'b', type: 'source' })
    expect(canConnect(from, 0, to, [], defForType)).toBe(false)
  })

  it('rejects an out-of-range source port index', () => {
    const from = node({ id: 'a', type: 'processor' }) // only port 0 exists
    const to = node({ id: 'b', type: 'processor' })
    expect(canConnect(from, 1, to, [], defForType)).toBe(false)
    expect(canConnect(from, -1, to, [], defForType)).toBe(false)
  })

  it('allows a port index within a multi-output node\'s range', () => {
    const from = node({ id: 'a', type: 'two-out' })
    const to = node({ id: 'b', type: 'processor' })
    expect(canConnect(from, 0, to, [], defForType)).toBe(true)
    expect(canConnect(from, 1, to, [], defForType)).toBe(true)
    expect(canConnect(from, 2, to, [], defForType)).toBe(false)
  })

  it('rejects a duplicate wire', () => {
    const from = node({ id: 'a', type: 'processor' })
    const to = node({ id: 'b', type: 'processor' })
    const existing: Wire[] = [{ from: 'a', out: 0, to: 'b' }]
    expect(canConnect(from, 0, to, existing, defForType)).toBe(false)
  })

  it('treats a missing `out` as 0 when checking for a duplicate', () => {
    const from = node({ id: 'a', type: 'processor' })
    const to = node({ id: 'b', type: 'processor' })
    const existing: Wire[] = [{ from: 'a', to: 'b' }] // out omitted, defaults to 0
    expect(canConnect(from, 0, to, existing, defForType)).toBe(false)
  })

  it('allows a second wire from the same pair on a different output port', () => {
    const from = node({ id: 'a', type: 'two-out' })
    const to = node({ id: 'b', type: 'processor' })
    const existing: Wire[] = [{ from: 'a', out: 0, to: 'b' }]
    expect(canConnect(from, 1, to, existing, defForType)).toBe(true)
  })

  it('rejects an unknown node type on either end', () => {
    const known = node({ id: 'b', type: 'processor' })
    const unknown = node({ id: 'a', type: 'missing' })
    expect(canConnect(unknown, 0, known, [], defForType)).toBe(false)
    expect(canConnect(known, 0, unknown, [], defForType)).toBe(false)
  })
})
