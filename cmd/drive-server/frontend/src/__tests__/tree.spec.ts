import { describe, expect, it } from 'vitest'
import { folderContents } from '../lib/entries'
import { spaceFromKey } from '../lib/space'
import { ancestorDirs, buildTree, documentLabel, findFolder, type TreeNode } from '../lib/tree'
import { doc } from './fixtures'

const docs = [
  doc({ path: 'readme.md', title: 'Read me' }),
  doc({ path: 'docs/10-faq.md', title: 'FAQ' }),
  doc({ path: 'docs/2-setup.md', title: 'Setup guide' }),
  doc({ path: 'docs/api/auth.html', kind: 'html', title: 'Auth' }),
  doc({ path: 'archive/old.md', title: 'Old notes', size: 100, updated_at: '2026-01-01T00:00:00Z' }),
]

function outline(root: TreeNode): string[] {
  const walk = (nodes: TreeNode[], depth: number): string[] =>
    nodes.flatMap((node) => [`${depth}:${node.name}`, ...walk(node.children, depth + 1)])
  return walk(root.children, 0)
}

describe('buildTree', () => {
  it('nests folders, puts them first, and sorts names naturally', () => {
    const rows = buildTree(docs)

    expect(outline(rows)).toEqual([
      '0:archive',
      '1:old.md',
      '0:docs',
      '1:api',
      '2:auth.html',
      '1:2-setup.md',
      '1:10-faq.md',
      '0:readme.md',
    ])
  })

  it('labels a document by its title and a folder by its name', () => {
    const root = buildTree(docs)

    expect(documentLabel(root.children.find((n) => n.name === 'readme.md')!.doc!)).toBe('Read me')
    expect(root.children.find((n) => n.name === 'docs')?.name).toBe('docs')
  })

  it('falls back to the file name without a title', () => {
    const root = buildTree([doc({ path: 'a/b.md', title: '' })])

    expect(documentLabel(findFolder(root, 'a')!.children[0].doc!)).toBe('b.md')
  })
})

describe('tree helpers', () => {
  it('lists the folders above a path', () => {
    expect(ancestorDirs('docs/api/auth.html')).toEqual(['docs', 'docs/api'])
    expect(ancestorDirs('readme.md')).toEqual([])
  })
})

describe('folderContents', () => {
  const space = spaceFromKey('acme/notes')

  it('lists the subfolders first, then the documents of a folder', () => {
    const entries = folderContents(docs, space, 'docs')!

    expect(entries.map((e) => `${e.kind}:${e.label}`)).toEqual(['dir:api', 'doc:Setup guide', 'doc:FAQ'])
    expect(entries[1].secondary).toBe('2-setup.md')
  })

  it('summarises a folder by its documents', () => {
    const root = folderContents(docs, space, '')!
    const archive = root.find((e) => e.label === 'archive')!

    expect(archive.secondary).toBe('1 document')
    expect(archive.size).toBe(100)
    expect(archive.modified).toBe('2026-01-01T00:00:00Z')
    expect(root.find((e) => e.label === 'docs')?.secondary).toBe('3 documents')
  })

  it('returns null for a folder with no documents', () => {
    expect(folderContents(docs, space, 'missing')).toBeNull()
  })
})
