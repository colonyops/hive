import { describe, expect, it } from 'vitest'
import type { Component } from 'vue'
import { documentIcon, folderIcon } from '../driveIcon'

const nameOf = (icon: Component) => (icon as { displayName?: string }).displayName

describe('driveIcon', () => {
  it('picks a document icon by drive kind before extension, and falls back to a plain file', () => {
    expect(nameOf(documentIcon('notes.txt', 'markdown'))).toBe('MaterialIcon(markdown)')
    expect(nameOf(documentIcon('page', 'html'))).toBe('MaterialIcon(html)')
    expect(nameOf(documentIcon('data.JSON', 'file'))).toBe('MaterialIcon(json)')
    expect(nameOf(documentIcon('notes.unknown', 'file'))).toBe('MaterialIcon(document)')
    expect(nameOf(documentIcon('noextension', 'file'))).toBe('MaterialIcon(document)')
  })

  it('tells source files apart by language', () => {
    expect(nameOf(documentIcon('main.go', ''))).toBe('MaterialIcon(go)')
    expect(nameOf(documentIcon('App.vue', ''))).toBe('MaterialIcon(vue)')
    expect(nameOf(documentIcon('index.ts', ''))).toBe('MaterialIcon(typescript)')
  })

  it('prefers the longest known extension', () => {
    expect(nameOf(documentIcon('model.spec.ts', ''))).toBe('MaterialIcon(test-ts)')
    expect(nameOf(documentIcon('env.d.ts', ''))).toBe('MaterialIcon(typescript-def)')
  })

  it('falls back to the base icon when the collection lacks a variant', () => {
    expect(nameOf(documentIcon('App.tsx', ''))).toBe('MaterialIcon(react_ts)')
    expect(documentIcon('App.tsx', '')).toBe(documentIcon('App.tsx', ''))
  })

  it('knows a file by its whole name, in any case', () => {
    expect(nameOf(documentIcon('Dockerfile', ''))).toBe('MaterialIcon(docker)')
    expect(nameOf(documentIcon('go.mod', ''))).toBe('MaterialIcon(go-mod)')
    expect(nameOf(documentIcon('MAKEFILE', ''))).toBe('MaterialIcon(makefile)')
  })

  it('gives a folder an open and a closed icon, named or not', () => {
    expect(nameOf(folderIcon('src', false))).toBe('MaterialIcon(folder-src)')
    expect(nameOf(folderIcon('SRC', true))).toBe('MaterialIcon(folder-src-open)')
    expect(nameOf(folderIcon('plans', false))).toBe('MaterialIcon(folder-tasks)')
    expect(nameOf(folderIcon('unnamed-thing', false))).toBe('MaterialIcon(folder-base)')
    expect(nameOf(folderIcon('unnamed-thing', true))).toBe('MaterialIcon(folder-base-open)')
  })
})
