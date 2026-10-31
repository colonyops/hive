import { describe, expect, it } from 'vitest'
import { filterAppletRows, reselect } from '../applets'

const columns = [{ key: 'name', label: 'Name', tones: {} }]
const rows = [{ name: 'lint' }, { name: 'test' }, { name: 'desktop:test' }]

describe('filterAppletRows', () => {
  it('keeps the command order with no query', () => {
    expect(filterAppletRows(rows, columns, ' ')).toEqual(rows)
  })

  it('drops what does not match and ranks the rest', () => {
    const matched = filterAppletRows(rows, columns, 'test')
    expect(matched.map((row) => row.name)).toEqual(['test', 'desktop:test'])
  })

  it('only searches shown columns', () => {
    expect(filterAppletRows([{ name: 'a', secret: 'hidden' }], columns, 'hidden')).toEqual([])
  })
})

describe('reselect', () => {
  it('follows the row by its first column', () => {
    expect(reselect([{ name: 'b' }, { name: 'a' }], columns, { name: 'a' }, 0)).toBe(1)
  })

  it('clamps when the row is gone', () => {
    expect(reselect([{ name: 'b' }], columns, { name: 'a' }, 5)).toBe(0)
    expect(reselect([], columns, undefined, 3)).toBe(0)
  })
})
