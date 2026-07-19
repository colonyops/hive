import { describe, expect, it } from 'vitest'
import { inDegrees, outWiresByPort, topoSort } from '../graph'
import type { Flow } from '../../types'

function flow(nodeIds: string[], wires: Array<{ from: string; to: string; out?: number }>): Flow {
  return { id: 'f', nodes: nodeIds.map((id) => ({ id, type: 'x', config: {} })), wires }
}

describe('graph helpers', () => {
  it('topoSort orders a linear chain', () => {
    const f = flow(['a', 'b', 'c'], [{ from: 'a', to: 'b' }, { from: 'b', to: 'c' }])
    expect(topoSort(f)).toEqual(['a', 'b', 'c'])
  })

  it('topoSort places independent branches after their shared source', () => {
    const f = flow(['src', 'x', 'y'], [{ from: 'src', to: 'x' }, { from: 'src', to: 'y' }])
    const order = topoSort(f)
    expect(order.indexOf('src')).toBeLessThan(order.indexOf('x'))
    expect(order.indexOf('src')).toBeLessThan(order.indexOf('y'))
  })

  it('topoSort throws instead of hanging on a cycle', () => {
    const f = flow(['a', 'b'], [{ from: 'a', to: 'b' }, { from: 'b', to: 'a' }])
    expect(() => topoSort(f)).toThrow(/cycle/i)
  })

  it('inDegrees counts inbound wires per node, ignoring dangling refs', () => {
    const f = flow(['a', 'b', 'c'], [{ from: 'a', to: 'c' }, { from: 'b', to: 'c' }, { from: 'ghost', to: 'a' }])
    const deg = inDegrees(f)
    expect(deg.get('a')).toBe(0)
    expect(deg.get('b')).toBe(0)
    expect(deg.get('c')).toBe(2)
  })

  it('outWiresByPort groups wires by source node and port', () => {
    const f = flow(['a', 'b', 'c'], [{ from: 'a', to: 'b', out: 0 }, { from: 'a', to: 'c', out: 1 }])
    const index = outWiresByPort(f)
    expect(index.get('a')?.get(0)?.map((w) => w.to)).toEqual(['b'])
    expect(index.get('a')?.get(1)?.map((w) => w.to)).toEqual(['c'])
  })

  it('outWiresByPort drops wires referencing unknown node ids', () => {
    const f = flow(['a'], [{ from: 'a', to: 'ghost' }, { from: 'ghost', to: 'a' }])
    const index = outWiresByPort(f)
    expect(index.size).toBe(0)
  })
})
