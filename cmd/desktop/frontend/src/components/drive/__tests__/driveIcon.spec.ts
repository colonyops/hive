import { describe, expect, it } from 'vitest'
import { documentIcon, folderIcon } from '../driveIcon'

describe('driveIcon', () => {
  it('picks a document icon by kind before extension, and falls back to a plain file', () => {
    expect(documentIcon('notes.md', 'markdown').color).toBe('text-node-blue')
    expect(documentIcon('perf.canvas.json', 'canvas').color).toBe('text-node-purple')
    expect(documentIcon('data.JSON', 'file').color).toBe('text-severity-warning')
    expect(documentIcon('notes.unknown', 'file').color).toBe('text-text-4')
    expect(documentIcon('noextension', 'file').color).toBe('text-text-4')
    expect(documentIcon('.hidden', 'file').color).toBe('text-text-4')
  })

  it('tells source files apart by language', () => {
    expect(documentIcon('main.go', '').color).toBe('text-node-teal')
    expect(documentIcon('App.vue', '').color).toBe('text-node-green')
    expect(documentIcon('index.ts', '').icon).toBe(documentIcon('main.go', '').icon)
  })

  it('knows a file with no useful extension by its whole name, in any case', () => {
    expect(documentIcon('Dockerfile', '').icon).not.toBe(documentIcon('x.unknown', '').icon)
    expect(documentIcon('.env', '').icon).toBe(documentIcon('server.pem', '').icon)
    expect(documentIcon('package-lock.json', '').icon).toBe(documentIcon('go.sum', '').icon)
    expect(documentIcon('MAKEFILE', '').icon).toBe(documentIcon('config.yaml', '').icon)
  })

  it('gives a named folder its own icon whether open or not, and any other folder an open and a closed one', () => {
    expect(folderIcon('Plans', false)).toBe(folderIcon('plans', true))
    expect(folderIcon('misc', false).icon).not.toBe(folderIcon('misc', true).icon)
  })
})
