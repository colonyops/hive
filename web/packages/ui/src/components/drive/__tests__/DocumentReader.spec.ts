import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DocumentReader from '../DocumentReader.vue'
import type { ReaderDocument } from '../../../lib/document'

const mermaid = vi.hoisted(() => ({
  render: vi.fn().mockResolvedValue('<svg viewBox="0 0 100 40"><text>Request</text></svg>'),
}))
vi.mock('../../../lib/mermaid', () => ({ renderMermaid: mermaid.render }))

function markdownDocument(markdown: string, front: Record<string, unknown> = {}, path = 'plans/x.md'): ReaderDocument {
  return { entry: { path, kind: 'markdown', front: front as never }, markdown }
}

async function mountReader(document: ReaderDocument, extra: Record<string, unknown> = {}) {
  const wrapper = mount(DocumentReader, {
    props: { document, fontSizePx: 13.5, lineHeight: 1.65, testid: 'r', ...extra },
  })
  await flushPromises()
  return wrapper
}

describe('DocumentReader', () => {
  beforeEach(() => {
    mermaid.render.mockReset().mockResolvedValue('<svg viewBox="0 0 100 40"><text>Request</text></svg>')
  })

  it('renders a markdown document without its front matter, which shows as properties', async () => {
    const wrapper = await mountReader(
      markdownDocument('---\ntype: plan\ntitle: The Plan\n---\n\n# The Plan\n\nBody.', {
        type: 'plan',
        title: 'The Plan',
      }),
    )

    const body = wrapper.get('[data-testid="r-markdown"]')
    expect(body.find('h1').text()).toBe('The Plan')
    expect(body.text()).not.toContain('type: plan')
    expect(wrapper.get('[data-testid="r-frontmatter-type"]').text()).toContain('plan')
    expect(wrapper.find('[data-testid="r-frame"]').exists()).toBe(false)
  })

  it('collapses the properties and remembers it', async () => {
    const wrapper = await mountReader(markdownDocument('# X', { type: 'plan' }))
    await wrapper.get('[data-testid="r-frontmatter-toggle"]').trigger('click')

    expect(wrapper.find('[data-testid="r-frontmatter-type"]').exists()).toBe(false)
    expect(localStorage.getItem('hive.canvas.frontmatter.expanded')).toBe('false')
  })

  // Agent-authored markdown is untrusted: raw HTML must arrive escaped, never
  // as live elements in the webview.
  it('renders markdown with raw HTML escaped', async () => {
    const wrapper = await mountReader(
      markdownDocument('# Plan\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>'),
    )
    const body = wrapper.get('[data-testid="r-markdown"]')
    expect(body.find('script').exists()).toBe(false)
    expect(body.find('img').exists()).toBe(false)
    expect(body.text()).toContain('<script>alert(1)</script>')
  })

  it('runs an html document in a frame sandboxed to scripts alone', async () => {
    const wrapper = await mountReader({
      entry: { path: 'page.html', kind: 'html' },
      html: '<html><head></head><body>Hi</body></html>',
    })

    const frame = wrapper.get('[data-testid="r-frame"]')
    expect(frame.attributes('sandbox')).toBe('allow-scripts')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(frame.attributes('srcdoc')).toContain('<body>Hi</body>')
    expect(frame.attributes('src')).toBeUndefined()
    expect(wrapper.get('[data-testid="r-document"]').classes()).toContain('absolute')
    expect(wrapper.find('[data-testid="r-frontmatter"]').exists()).toBe(false)
  })

  it('loads an html document from htmlSrc instead of inlining it', async () => {
    const wrapper = await mountReader(
      { entry: { path: 'page.html', kind: 'html' }, html: '<body>Hi</body>' },
      { htmlSrc: '/raw/page.html' },
    )

    const frame = wrapper.get('[data-testid="r-frame"]')
    expect(frame.attributes('src')).toBe('/raw/page.html')
    expect(frame.attributes('srcdoc')).toBeUndefined()
    expect(frame.attributes('sandbox')).toBe('allow-scripts')
  })

  it('shows an error over the document', async () => {
    const failed = await mountReader(markdownDocument('# X'), { error: 'Could not read the document.' })
    expect(failed.get('[data-testid="r-error"]').text()).toContain('Could not read the document.')
    expect(failed.find('[data-testid="r-markdown"]').exists()).toBe(false)
  })

  it('renders a fenced Mermaid diagram in markdown as a themed SVG', async () => {
    const wrapper = await mountReader(markdownDocument('```mermaid\nflowchart LR\nA --> B\n```'))
    expect(mermaid.render).toHaveBeenCalledWith('flowchart LR\nA --> B\n', expect.objectContaining({ darkMode: true }))
    expect(wrapper.get('[data-testid="r-markdown-mermaid-0"]').text()).toContain('Request')
  })

  it('applies the reading settings to the markdown body', async () => {
    const wrapper = await mountReader(markdownDocument('# Plan'), { fontSizePx: 18, lineHeight: 1.85 })
    const style = (wrapper.get('[data-testid="r-markdown"]').element as HTMLElement).style
    expect(style.getPropertyValue('--hv-font-size')).toBe('18px')
    expect(style.getPropertyValue('--hv-line-height')).toBe('1.85')
  })

  describe('links', () => {
    it('reports the target of a link in markdown', async () => {
      const wrapper = await mountReader(markdownDocument('[the PR](https://example.com/pr/1)'))
      await wrapper.get('[data-testid="r-markdown"] a').trigger('click')

      expect(wrapper.emitted('open-link')).toEqual([['https://example.com/pr/1']])
    })

    it('reports a relative link as written', async () => {
      const wrapper = await mountReader(
        markdownDocument('[report](../reports/report.md) [research](.hive/research/r.md)'),
      )
      for (const anchor of wrapper.findAll('[data-testid="r-markdown"] a')) await anchor.trigger('click')

      expect(wrapper.emitted('open-link')).toEqual([['../reports/report.md'], ['.hive/research/r.md']])
    })
  })
})
