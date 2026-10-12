import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DriveReader from '../DriveReader.vue'
import type {
  BlockWire,
  DocumentWire,
} from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { doc, documentFor } from '../../../test-utils/drive'

const mermaid = vi.hoisted(() => ({
  render: vi.fn().mockResolvedValue('<svg viewBox="0 0 100 40"><text>Request</text></svg>'),
}))
vi.mock('../../../lib/mermaid', () => ({ renderMermaid: mermaid.render }))

const settingsBindings = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
}))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settingsBindings,
)

const paths = new Set(['plans/x.md', 'canvases/report.canvas.json', 'research/r.md', 'page.html'])

function block(overrides: Partial<BlockWire>): BlockWire {
  return { id: 'b', kind: 'markdown', title: '', body: '', url: '', createdAt: 1, updatedAt: 1, ...overrides }
}

function canvasDocument(blocks: BlockWire[], frontmatter: Record<string, unknown> = {}): DocumentWire {
  return documentFor('acme/site', doc('canvases/report.canvas.json'), { canvas: { blocks, frontmatter } })
}

async function mountReader(document: DocumentWire | null, extra: Record<string, unknown> = {}) {
  const wrapper = mount(DriveReader, {
    props: { document, paths, loading: false, error: null, testid: 'r', ...extra },
  })
  await flushPromises()
  return wrapper
}

describe('DriveReader', () => {
  beforeEach(() => {
    localStorage.clear()
    settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
    mermaid.render.mockReset().mockResolvedValue('<svg viewBox="0 0 100 40"><text>Request</text></svg>')
  })

  it('renders a markdown document without its front matter, which shows as properties', async () => {
    const entry = doc('plans/x.md', { title: 'The Plan', front: { type: 'plan', title: 'The Plan' } })
    const wrapper = await mountReader(
      documentFor('acme/site', entry, { markdown: '---\ntype: plan\ntitle: The Plan\n---\n\n# The Plan\n\nBody.' }),
    )

    const body = wrapper.get('[data-testid="r-markdown"]')
    expect(body.find('h1').text()).toBe('The Plan')
    expect(body.text()).not.toContain('type: plan')
    expect(wrapper.get('[data-testid="r-frontmatter-type"]').text()).toContain('plan')
    expect(wrapper.find('[data-testid="r-frame"]').exists()).toBe(false)
  })

  // Agent-authored markdown is untrusted: raw HTML must arrive escaped, never
  // as live elements in the webview.
  it('renders markdown with raw HTML escaped', async () => {
    const wrapper = await mountReader(
      documentFor('acme/site', doc('plans/x.md'), {
        markdown: '# Plan\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>',
      }),
    )
    const body = wrapper.get('[data-testid="r-markdown"]')
    expect(body.find('script').exists()).toBe(false)
    expect(body.find('img').exists()).toBe(false)
    expect(body.text()).toContain('<script>alert(1)</script>')
  })

  it('runs an html document in a frame sandboxed to scripts alone', async () => {
    const wrapper = await mountReader(
      documentFor('acme/site', doc('page.html'), { html: '<html><head></head><body>Hi</body></html>' }),
    )

    const frame = wrapper.get('[data-testid="r-frame"]')
    expect(frame.attributes('sandbox')).toBe('allow-scripts')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(frame.attributes('srcdoc')).toContain('<body>Hi</body>')
    expect(wrapper.get('[data-testid="r-frame-bar"]').text()).toContain('cannot reach Hive or localhost')
    expect(wrapper.find('[data-testid="r-frontmatter"]').exists()).toBe(false)
  })

  it('says a plain file is not shown', async () => {
    const wrapper = await mountReader(documentFor('acme/site', doc('notes.txt')))
    expect(wrapper.find('[data-testid="r-file"]').exists()).toBe(true)
  })

  it('shows the empty state with no document, and an error over everything', async () => {
    const empty = await mountReader(null)
    expect(empty.find('[data-testid="r-empty"]').exists()).toBe(true)

    const failed = await mountReader(null, { error: 'Could not read the document.' })
    expect(failed.get('[data-testid="r-error"]').text()).toContain('Could not read the document.')
    expect(failed.find('[data-testid="r-empty"]').exists()).toBe(false)
  })

  // An html block's body is sanitized on the Go read path, so the reader
  // renders it as markup under its own class scope, never through the
  // markdown renderer, which is shared with untrusted GitHub bodies.
  it('renders a canvas block by block, html blocks as live markup under the hv-html scope', async () => {
    const wrapper = await mountReader(
      canvasDocument(
        [
          block({ id: 'doc', body: '# Plan' }),
          block({
            id: 'stats',
            kind: 'html',
            title: 'Run',
            body: '<div class="hv-stat"><span class="hv-stat-value">42</span></div>',
          }),
        ],
        { tags: ['release'] },
      ),
    )

    expect(wrapper.get('[data-testid="r-block-doc"] h1').text()).toBe('Plan')
    const rendered = wrapper.get('[data-testid="r-block-stats"] .hv-html')
    expect(rendered.find('.hv-stat-value').text()).toBe('42')
    expect(rendered.classes()).not.toContain('markdown-body')
    expect(wrapper.get('[data-testid="r-block-stats"] h2').text()).toBe('Run')
    expect(wrapper.get('[data-testid="r-frontmatter-tags"]').text()).toContain('release')
  })

  it("renders an html block's svg as real svg nodes", async () => {
    const wrapper = await mountReader(
      canvasDocument([
        block({
          id: 'flow',
          kind: 'html',
          body: '<svg viewBox="0 0 200 60"><rect class="hv-node" x="1" y="1" width="70" height="34"/></svg>',
        }),
      ]),
    )
    const svg = wrapper.get('[data-testid="r-block-flow"] .hv-html svg')
    expect(svg.element.namespaceURI).toBe('http://www.w3.org/2000/svg')
    expect(svg.element.getAttribute('viewBox')).toBe('0 0 200 60')
  })

  it('renders a fenced Mermaid diagram in markdown as a themed SVG', async () => {
    const wrapper = await mountReader(
      canvasDocument([block({ id: 'flow', body: '```mermaid\nflowchart LR\nA --> B\n```' })]),
    )
    expect(mermaid.render).toHaveBeenCalledWith('flowchart LR\nA --> B\n', expect.objectContaining({ darkMode: true }))
    expect(wrapper.get('[data-testid="r-block-flow-mermaid-0"]').text()).toContain('Request')
  })

  it('applies the reading settings to markdown and html bodies', async () => {
    settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'xl', canvasLineSpacing: 'relaxed' })
    const wrapper = await mountReader(
      canvasDocument([block({ id: 'doc', body: '# Plan' }), block({ id: 'stats', kind: 'html', body: '<p>42</p>' })]),
    )
    for (const body of wrapper.findAll('.canvas-reading-body')) {
      const style = (body.element as HTMLElement).style
      expect(style.getPropertyValue('--hv-font-size')).toBe('18px')
      expect(style.getPropertyValue('--hv-line-height')).toBe('1.85')
    }
  })

  describe('links', () => {
    it('opens web links outside the app from markdown, html blocks, and link blocks', async () => {
      const wrapper = await mountReader(
        canvasDocument([
          block({ id: 'doc', body: '[the PR](https://example.com/pr/1)' }),
          block({ id: 'card', kind: 'html', body: '<p><a href="https://example.com/pr/2">PR 2</a></p>' }),
          block({ id: 'ok', kind: 'link', title: 'PR 3', url: 'https://example.com/pr/3' }),
          block({ id: 'bad', kind: 'link', title: 'Nope', url: 'javascript:alert(1)' }),
        ]),
      )
      await wrapper.get('[data-testid="r-block-doc"] a').trigger('click')
      await wrapper.get('[data-testid="r-block-card"] a').trigger('click')
      await wrapper.get('[data-testid="r-block-ok"] button').trigger('click')
      await wrapper.get('[data-testid="r-block-bad"] button').trigger('click')

      expect(wrapper.emitted('open-url')).toEqual([
        ['https://example.com/pr/1'],
        ['https://example.com/pr/2'],
        ['https://example.com/pr/3'],
      ])
      expect(wrapper.emitted('open')).toBeUndefined()
    })

    it('opens a document a relative or .hive/ link names, and nothing for an unknown one', async () => {
      const wrapper = await mountReader(
        documentFor('acme/site', doc('plans/x.md'), {
          markdown:
            '[report](../canvases/report.canvas.json) [research](.hive/research/r.md) [typo](../canvases/reprot.canvas.json)',
        }),
      )
      for (const anchor of wrapper.findAll('[data-testid="r-markdown"] a')) await anchor.trigger('click')

      expect(wrapper.emitted('open')).toEqual([['canvases/report.canvas.json'], ['research/r.md']])
      expect(wrapper.emitted('open-url')).toBeUndefined()
    })
  })
})
