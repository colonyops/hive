import { describe, expect, it } from 'vitest'
import { moveId } from '../listOrder'

describe('moveId', () => {
  const ids = ['a', 'b', 'c']

  it('moves an item to either edge of another', () => {
    expect(moveId(ids, 'a', { id: 'c', edge: 'after' })).toEqual(['b', 'c', 'a'])
    expect(moveId(ids, 'c', { id: 'a', edge: 'before' })).toEqual(['c', 'a', 'b'])
    expect(moveId(ids, 'a', { id: 'c', edge: 'before' })).toEqual(['b', 'a', 'c'])
  })

  it('returns null for moves that would not change the order', () => {
    expect(moveId(ids, 'a', { id: 'a', edge: 'before' })).toBeNull()
    expect(moveId(ids, 'a', { id: 'b', edge: 'before' })).toBeNull()
    expect(moveId(ids, 'b', { id: 'a', edge: 'after' })).toBeNull()
  })

  it('returns null when either end of the move is unknown', () => {
    expect(moveId(ids, 'ghost', { id: 'a', edge: 'after' })).toBeNull()
    expect(moveId(ids, 'a', { id: 'ghost', edge: 'after' })).toBeNull()
  })
})
