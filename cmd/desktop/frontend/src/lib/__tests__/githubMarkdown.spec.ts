import { describe, expect, it } from 'vitest'
import { renderCanvasMarkdown, renderGithubMarkdown } from '../githubMarkdown'
import { useCodeHighlight } from '../../stores/useCodeHighlight'

describe('renderGithubMarkdown', () => {
  it('renders a fence escaped with no highlighter, then highlighted once the queue drains', async () => {
    const source = '```go\nfunc main() {}\n```'
    expect(renderGithubMarkdown(source)).toContain('<pre><code class="language-go">func main() {}\n</code></pre>')

    const { highlight, load, settle } = useCodeHighlight()
    await load()
    expect(renderGithubMarkdown(source, highlight)).toContain('<pre><code class="language-go">')
    await settle()
    const html = renderGithubMarkdown(source, highlight)
    expect(html).toContain('class="shiki')
    expect(html).toContain('--shiki-app')
  })

  it('wraps every code block in a block that carries a copy button', () => {
    const fenced = renderGithubMarkdown('```go\nfunc main() {}\n```')
    expect(fenced).toContain(
      '<div class="markdown-code"><button type="button" class="markdown-code-copy" data-testid="markdown-code-copy" aria-label="Copy code"',
    )
    expect(fenced).toContain('</button><pre><code class="language-go">func main() {}\n</code></pre>\n</div>')

    const indented = renderGithubMarkdown('    indented code\n')
    expect(indented).toContain('class="markdown-code-copy"')
    expect(indented).toContain('</button><pre><code>indented code\n</code></pre>\n</div>')
  })

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
    const src =
      'You can ask for more help in #proj-renovate-self-hosted.\n<!--renovate-debug:eyJ2ZXIiOiI0My4yNjYuMCJ9-->'
    const html = renderGithubMarkdown(src)
    expect(html).toContain('You can ask for more help in #proj-renovate-self-hosted.')
    expect(html).not.toContain('renovate-debug')
    expect(html).not.toContain('&lt;!--')
  })

  it('preserves comment-like text inside code', async () => {
    const source = '`<!-- keep me -->`\n\n```html\n<!-- keep me too -->\n```'
    const escaped = renderGithubMarkdown(source)
    expect(escaped).toContain('&lt;!-- keep me --&gt;')
    expect(escaped).toContain('&lt;!-- keep me too --&gt;')

    const { highlight, load, settle } = useCodeHighlight()
    await load()
    renderGithubMarkdown(source, highlight)
    await settle()
    const highlighted = renderGithubMarkdown(source, highlight)
    expect(highlighted).toContain('&lt;!-- keep me --&gt;')
    expect(highlighted).toContain('&#x3C;!-- keep me too -->')
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

describe('renderCanvasMarkdown', () => {
  it('splits standalone Mermaid fences from surrounding markdown', () => {
    expect(renderCanvasMarkdown('Before\n\n```mermaid\nflowchart LR\nA --> B\n```\n\nAfter')).toEqual([
      { kind: 'html', html: '<p>Before</p>\n' },
      { kind: 'mermaid', source: 'flowchart LR\nA --> B\n' },
      { kind: 'html', html: '<p>After</p>\n' },
    ])
  })

  it('leaves non-Mermaid and nested fences as code', () => {
    const parts = renderCanvasMarkdown('```js\nconst x = 1\n```\n\n> ```mermaid\n> A --> B\n> ```')
    expect(parts).toHaveLength(1)
    expect(parts[0]).toMatchObject({ kind: 'html' })
    expect((parts[0] as { html: string }).html).toContain('language-mermaid')
  })

  it('does not change the shared GitHub markdown renderer', () => {
    expect(renderGithubMarkdown('```mermaid\nA --> B\n```')).toContain('language-mermaid')
  })
})

describe('renderCanvasMarkdown source lines', () => {
  const html = (src: string, offset = 0) =>
    renderCanvasMarkdown(src, undefined, { offset })
      .map((part) => (part.kind === 'html' ? part.html : ''))
      .join('')

  it('marks headings, paragraphs, blockquotes and rules with their lines', () => {
    const out = html('# Title\n\nOne\ntwo\n\n> quoted\n\n---\n')
    expect(out).toContain('<h1 data-source-start="1" data-source-end="1">Title</h1>')
    expect(out).toContain('<p data-source-start="3" data-source-end="4">One')
    expect(out).toContain('<blockquote data-source-start="6" data-source-end="6">')
    expect(out).toContain('<hr data-source-start="8" data-source-end="8">')
  })

  it('marks nested list items and tables', () => {
    const out = html('- a\n  - b\n- c\n\n| x |\n| - |\n| 1 |\n')
    expect(out).toContain('<ul data-source-start="1" data-source-end="3">')
    expect(out).toContain('<li data-source-start="2" data-source-end="2">b</li>')
    expect(out).toContain('<table data-source-start="5" data-source-end="7">')
  })

  it('puts a fence range on its copy wrapper, highlighted or not', async () => {
    const source = 'Intro\n\n```go\nfunc main() {}\n```\n'
    expect(html(source)).toContain('<div class="markdown-code" data-source-start="3" data-source-end="5">')

    const { highlight, load, settle } = useCodeHighlight()
    await load()
    renderCanvasMarkdown(source, highlight)
    await settle()
    const highlighted = renderCanvasMarkdown(source, highlight, { offset: 0 })
      .map((part) => (part.kind === 'html' ? part.html : ''))
      .join('')
    expect(highlighted).toContain('class="shiki')
    expect(highlighted).toContain('<div class="markdown-code" data-source-start="3" data-source-end="5">')
  })

  it('wraps escaped html blocks', () => {
    expect(html('<section>raw</section>\n')).toContain(
      '<div data-source-start="1" data-source-end="1">&lt;section&gt;raw&lt;/section&gt;\n</div>',
    )
  })

  it('gives mermaid parts their lines', () => {
    const parts = renderCanvasMarkdown('Before\n\n```mermaid\ngraph TD\n```\n', undefined, { offset: 2 })
    expect(parts[1]).toEqual({ kind: 'mermaid', source: 'graph TD\n', lines: [5, 7] })
  })

  it('adds the offset and trims trailing blank lines', () => {
    expect(html('- a\n\n\n- b\n', 3)).toContain('<li data-source-start="4" data-source-end="4">')
  })

  it('leaves the output unchanged without sourceLines', () => {
    const source = '# T\n\n- a\n\n```go\nx\n```\n\n<b>x</b>\n\n---\n\n```mermaid\ngraph\n```\n'
    const parts = renderCanvasMarkdown(source)
    expect(JSON.stringify(parts)).not.toContain('data-source')
    expect(parts.find((part) => part.kind === 'mermaid')).toEqual({ kind: 'mermaid', source: 'graph\n' })
    expect(renderGithubMarkdown(source)).not.toContain('data-source')
  })
})
