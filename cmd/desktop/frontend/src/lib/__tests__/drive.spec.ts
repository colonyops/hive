import { describe, expect, it } from 'vitest'
import {
  ancestorPaths,
  documentsOf,
  driveEventAuthor,
  entryMatches,
  frontmatterLineCount,
  linkTarget,
  markdownBody,
  newestDocument,
  spaceGroup,
} from '../drive'
import { doc, folder, root } from '../../test-utils/drive'

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

  it('strips front matter from a markdown body and leaves a body without it alone', () => {
    expect(markdownBody('---\ntype: plan\ntitle: X\n---\n\n# X\n')).toBe('\n# X\n')
    expect(markdownBody('# X\n\n---\n\nmore')).toBe('# X\n\n---\n\nmore')
  })

  it('counts the lines front matter takes, LF or CRLF', () => {
    expect(frontmatterLineCount('---\ntype: plan\n---\n\n# X\n')).toBe(3)
    expect(frontmatterLineCount('---\r\ntype: plan\r\ntitle: X\r\n---\r\n# X\r\n')).toBe(4)
    expect(frontmatterLineCount('---\na: 1\n---')).toBe(2)
    expect(frontmatterLineCount('# X\n')).toBe(0)
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

  describe('linkTarget', () => {
    const paths = new Set(['plans/x.md', 'canvases/report.canvas.json', 'research/r.md'])

    it('opens web and mail links outside the app', () => {
      expect(linkTarget('https://example.com', 'plans/x.md', paths)).toEqual({
        kind: 'url',
        url: 'https://example.com',
      })
      expect(linkTarget('mailto:a@b.c', 'plans/x.md', paths)).toEqual({ kind: 'url', url: 'mailto:a@b.c' })
    })

    it('resolves a relative link against the linking document', () => {
      expect(linkTarget('../canvases/report.canvas.json', 'plans/x.md', paths)).toEqual({
        kind: 'document',
        path: 'canvases/report.canvas.json',
      })
      expect(linkTarget('./x.md#scope', 'plans/y.md', paths)).toEqual({ kind: 'document', path: 'plans/x.md' })
    })

    it('resolves a .hive/ link from the space root', () => {
      expect(linkTarget('.hive/research/r.md', 'plans/x.md', paths)).toEqual({
        kind: 'document',
        path: 'research/r.md',
      })
    })

    it('finds a canvas by its bare name', () => {
      expect(linkTarget('report', 'canvases/other.canvas.json', paths)).toEqual({
        kind: 'document',
        path: 'canvases/report.canvas.json',
      })
    })

    it('leads nowhere for an unknown document, an escape, or another scheme', () => {
      expect(linkTarget('missing.md', 'plans/x.md', paths)).toBeNull()
      expect(linkTarget('../../x.md', 'plans/x.md', paths)).toBeNull()
      expect(linkTarget('javascript:alert(1)', 'plans/x.md', paths)).toBeNull()
      expect(linkTarget('/etc/passwd', 'plans/x.md', paths)).toBeNull()
    })
  })
})
