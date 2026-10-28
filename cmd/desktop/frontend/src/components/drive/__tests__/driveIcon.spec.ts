import { describe, expect, it } from 'vitest'
import { documentIcon, folderIcon } from '../driveIcon'

describe('driveIcon', () => {
  it('picks a document icon by kind before extension, and falls back to a plain file', () => {
    expect(documentIcon('notes.md', 'markdown').color).toBe('text-node-blue')
    expect(documentIcon('perf.canvas.json', 'canvas').color).toBe('text-node-purple')
    expect(documentIcon('data.JSON', 'file').color).toBe('text-severity-warning')
    expect(documentIcon('Makefile', 'file').color).toBe('text-text-4')
    expect(documentIcon('.env', 'file').color).toBe('text-text-4')
  })

  it('gives a named folder its own icon whether open or not, and any other folder an open and a closed one', () => {
    expect(folderIcon('Plans', false)).toBe(folderIcon('plans', true))
    expect(folderIcon('misc', false).icon).not.toBe(folderIcon('misc', true).icon)
  })
})
