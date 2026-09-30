import { describe, expect, it } from 'vitest'
import type { Terminal } from '@xterm/xterm'
import { proposeGrid, terminalCellSize } from '../terminalGrid'

const cell = { width: 10, height: 25 }

function terminalWithCell(size: { width: number; height: number } | undefined): Terminal {
  const core = size === undefined ? {} : { _renderService: { dimensions: { css: { cell: size } } } }
  return { _core: core } as unknown as Terminal
}

describe('proposeGrid', () => {
  it('counts the whole cells a box holds', () => {
    expect(proposeGrid({ width: 1200, height: 1000 }, cell)).toEqual({ cols: 120, rows: 40 })
    expect(proposeGrid({ width: 1209, height: 1024 }, cell)).toEqual({ cols: 120, rows: 40 })
  })

  it('proposes nothing for a box narrower or shorter than one cell', () => {
    expect(proposeGrid({ width: 9, height: 1000 }, cell)).toBeNull()
    expect(proposeGrid({ width: 1200, height: 24 }, cell)).toBeNull()
  })

  it('proposes nothing for a box with no size', () => {
    expect(proposeGrid({ width: 0, height: 0 }, cell)).toBeNull()
    expect(proposeGrid({ width: 0, height: 1000 }, cell)).toBeNull()
  })

  it('proposes nothing before a cell has been measured', () => {
    expect(proposeGrid({ width: 1200, height: 1000 }, { width: 0, height: 0 })).toBeNull()
  })
})

describe('terminalCellSize', () => {
  it('reads the rendered cell', () => {
    expect(terminalCellSize(terminalWithCell({ width: 7.5, height: 17 }))).toEqual({ width: 7.5, height: 17 })
  })

  it('is null before a cell is measured', () => {
    expect(terminalCellSize(terminalWithCell({ width: 0, height: 0 }))).toBeNull()
  })

  it('is null when the private field it reads is missing', () => {
    expect(terminalCellSize(terminalWithCell(undefined))).toBeNull()
    expect(terminalCellSize({} as unknown as Terminal)).toBeNull()
  })
})
