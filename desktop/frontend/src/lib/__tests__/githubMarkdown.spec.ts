import { describe, expect, it } from 'vitest'
import { renderGithubMarkdown } from '../githubMarkdown'

describe('renderGithubMarkdown', () => {
  it('returns an empty string for blank input', () => {
    expect(renderGithubMarkdown('')).toBe('')
    expect(renderGithubMarkdown('   \n  ')).toBe('')
  })

  it('renders headings and paragraphs', () => {
    const html = renderGithubMarkdown('# Title\n\nHello world')
    expect(html).toContain('<h1>Title</h1>')
    expect(html).toContain('<p>Hello world</p>')
  })

  it('renders links', () => {
    const html = renderGithubMarkdown('see [the docs](https://example.com/docs)')
    expect(html).toContain('<a href="https://example.com/docs">the docs</a>')
  })

  it('renders GFM tables', () => {
    const src = ['| a | b |', '| - | - |', '| 1 | 2 |'].join('\n')
    const html = renderGithubMarkdown(src)
    expect(html).toContain('<table>')
    expect(html).toContain('<th>a</th>')
    expect(html).toContain('<td>1</td>')
  })

  it('renders GFM task lists as checkboxes', () => {
    const html = renderGithubMarkdown('- [x] done\n- [ ] todo')
    expect(html).toContain('type="checkbox"')
    expect(html).toContain('checked')
  })

  it('renders strikethrough', () => {
    expect(renderGithubMarkdown('~~gone~~')).toContain('<s>gone</s>')
  })

  it('renders collapsible details with markdown content', () => {
    const src = '<details>\n<summary>More information</summary>\n\n**Hidden** details\n\n</details>'
    const html = renderGithubMarkdown(src)
    expect(html).toContain('<details>')
    expect(html).toContain('<summary>More information</summary>')
    expect(html).toContain('<strong>Hidden</strong> details')
    expect(html).toContain('</details>')
  })

  it('supports open details but escapes unsupported attributes', () => {
    expect(renderGithubMarkdown('<details open>\ncontent\n</details>')).toContain('<details open="">')

    const html = renderGithubMarkdown('<details onclick="alert(1)">\ncontent\n</details>')
    expect(html).not.toContain('<details onclick')
    expect(html).toContain('&lt;details onclick=')
  })

  it.each([
    ['NOTE', 'Note'],
    ['TIP', 'Tip'],
    ['IMPORTANT', 'Important'],
    ['WARNING', 'Warning'],
    ['CAUTION', 'Caution'],
  ])('renders a GitHub %s alert', (type, label) => {
    const html = renderGithubMarkdown(`> [!${type}]\n> Useful **information**`)
    expect(html).toContain(`class="markdown-alert markdown-alert-${type.toLowerCase()}"`)
    expect(html).toContain(`<p class="markdown-alert-title">${label}</p>`)
    expect(html).toContain('Useful <strong>information</strong>')
    expect(html).not.toContain(`[!${type}]`)
  })

  it('removes GitHub HTML comments without hiding surrounding content', () => {
    const src = 'You can ask for more help in #proj-renovate-self-hosted.\n<!--renovate-debug:eyJ2ZXIiOiI0My4yNjYuMCJ9-->'
    const html = renderGithubMarkdown(src)
    expect(html).toContain('You can ask for more help in #proj-renovate-self-hosted.')
    expect(html).not.toContain('renovate-debug')
    expect(html).not.toContain('&lt;!--')
  })

  it('preserves comment-like text inside code', () => {
    const html = renderGithubMarkdown('`<!-- keep me -->`\n\n```html\n<!-- keep me too -->\n```')
    expect(html).toContain('&lt;!-- keep me --&gt;')
    expect(html).toContain('&lt;!-- keep me too --&gt;')
  })

  it('renders fenced code blocks', () => {
    const html = renderGithubMarkdown('```\nconst x = 1\n```')
    expect(html).toContain('<pre>')
    expect(html).toContain('const x = 1')
  })

  it('escapes raw HTML so a script tag never becomes a live element', () => {
    const html = renderGithubMarkdown('hi <script>alert(1)</script>')
    expect(html).not.toContain('<script>')
    expect(html).toContain('&lt;script&gt;')
  })

  it('escapes raw HTML with inline event handlers instead of emitting a tag', () => {
    const html = renderGithubMarkdown('<img src=x onerror="alert(1)">')
    expect(html).not.toContain('<img')
    expect(html).toContain('&lt;img')
  })

  it('does not turn a javascript: URL into a clickable link', () => {
    const html = renderGithubMarkdown('[click](javascript:alert(1))')
    expect(html).not.toMatch(/href=["']?javascript:/i)
    expect(html).not.toContain('<a ')
  })
})
