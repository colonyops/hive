import { describe, expect, it } from 'vitest'
import {
  ancestorPaths,
  documentsOf,
  driveEventAuthor,
  entryMatches,
  newestDocument,
  spaceGroup,
  toReaderDocument,
} from '../drive'
import { doc, documentFor, folder, root } from '../../test-utils/drive'

describe('drive', () => {
  it('matches a filter against the name or the title, ignoring case and padding', () => {
    const entry = doc('plans/auth.md', { title: 'Login Flow' })
    expect(entryMatches(entry, 'AUTH')).toBe(true)
    expect(entryMatches(entry, ' login ')).toBe(true)
    expect(entryMatches(entry, 'plans')).toBe(false)
    expect(entryMatches(entry, '')).toBe(true)
  })

  it('groups a space by its key', () => {
    expect(spaceGroup('colonyops/hive')).toBe('Repositories')
    expect(spaceGroup('workspaces/demo')).toBe('Workspaces')
    expect(spaceGroup('shared')).toBe('Shared')
  })

  it('names the author of a drive event, a hive session before a chat', () => {
    expect(driveEventAuthor({ session: '7', hiveSession: '' })).toBe('7')
    expect(driveEventAuthor([{ session: '', hiveSession: 'abc123' }])).toBe('abc123')
    expect(driveEventAuthor({ session: '', hiveSession: '' })).toBeNull()
    expect(driveEventAuthor(undefined)).toBeNull()
  })

  it('lists documents depth first and picks the newest', () => {
    const tree = root([
      folder('plans', [doc('plans/a.md', { modified: '2026-10-02T00:00:00Z' })]),
      doc('notes.md', { modified: '2026-10-03T00:00:00Z' }),
    ])
    expect(documentsOf(tree).map((entry) => entry.path)).toEqual(['plans/a.md', 'notes.md'])
    expect(newestDocument(tree)?.path).toBe('notes.md')
    expect(newestDocument(null)).toBeNull()
  })

  it('names the folders that hold a path', () => {
    expect(ancestorPaths('a/b/c.md')).toEqual(['a', 'a/b'])
    expect(ancestorPaths('c.md')).toEqual([])
  })

  describe('toReaderDocument', () => {
    it('maps a canvas, turning unix millisecond stamps into ISO ones and dropping the block stamps', () => {
      const wire = documentFor('acme/site', doc('c.canvas.json'), {
        canvas: {
          createdAt: Date.parse('2026-10-01T00:00:00Z'),
          updatedAt: 0,
          frontmatter: { tags: ['a'] },
          blocks: [{ id: 'b', kind: 'link', title: 'T', body: '', url: 'https://x.test', createdAt: 5, updatedAt: 6 }],
        },
      })
      expect(toReaderDocument(wire)?.canvas).toEqual({
        blocks: [{ id: 'b', kind: 'link', title: 'T', body: '', url: 'https://x.test' }],
        frontmatter: { tags: ['a'] },
        createdAt: '2026-10-01T00:00:00.000Z',
        updatedAt: undefined,
      })
    })

    it('carries a markdown entry with its front matter and times', () => {
      const wire = documentFor('acme/site', doc('plans/x.md', { front: { type: 'plan' } }), { markdown: '# X' })
      expect(toReaderDocument(wire)).toMatchObject({
        entry: { path: 'plans/x.md', kind: 'markdown', front: { type: 'plan' }, modified: '2026-10-01T00:00:00Z' },
        markdown: '# X',
        canvas: undefined,
      })
    })

    it('has no reader document for a plain file', () => {
      expect(toReaderDocument(documentFor('acme/site', doc('notes.txt')))).toBeNull()
    })
  })
})
