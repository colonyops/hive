import { describe, expect, it } from 'vitest'
import { frontmatterLineCount, linkTarget, markdownBody } from '../document'

describe('document', () => {
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
