import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentCanvasPane from '../AgentCanvasPane.vue'
import type { AgentWorkspacesClient, CanvasBlock, WorkspaceCanvasMeta } from '../../lib/agentWorkspacesClient'

const wailsEvents = vi.hoisted(() => ({
  handlers: [] as Array<[string, (event: { data: unknown }) => void]>,
  fire(name: string, data: unknown) {
    for (const [registered, handler] of this.handlers) {
      if (registered === name) handler({ data })
    }
  },
}))
vi.mock('../../composables/useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: (event: { data: unknown }) => void) => {
    wailsEvents.handlers.push([name, handler])
  },
}))

const runtime = vi.hoisted(() => ({
  setText: vi.fn().mockResolvedValue(undefined),
  saveFile: vi.fn().mockResolvedValue('/tmp/plan.md'),
}))
vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: runtime.setText },
  Dialogs: { SaveFile: runtime.saveFile },
}))

const mermaid = vi.hoisted(() => ({
  render: vi.fn().mockResolvedValue('<svg viewBox="0 0 100 40"><text>Request</text></svg>'),
}))
vi.mock('../../lib/mermaid', () => ({ renderMermaid: mermaid.render }))

const settingsBindings = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
}))
vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settingsBindings,
)

function block(overrides: Partial<CanvasBlock>): CanvasBlock {
  return { id: 'b', kind: 'markdown', title: '', body: '', url: '', createdAt: 1, updatedAt: 1, ...overrides }
}

function meta(overrides: Partial<WorkspaceCanvasMeta>): WorkspaceCanvasMeta {
  return {
    workspace: 'web-app',
    name: 'plan',
    title: '',
    session: '7',
    hiveSession: '',
    createdAt: 1,
    updatedAt: 1,
    blockCount: 1,
    ...overrides,
  }
}

function fakeCanvasClient(blocks: CanvasBlock[], metas: WorkspaceCanvasMeta[] = [meta({})]) {
  return {
    canvas: vi.fn().mockImplementation((workspace: string, name: string) =>
      Promise.resolve({
        workspace,
        name,
        title: '',
        session: '7',
        createdAt: 1,
        updatedAt: 2,
        frontmatter: { tags: ['release', 'desktop'] },
        blocks,
      }),
    ),
    canvases: vi.fn().mockResolvedValue(metas),
    deleteCanvas: vi.fn().mockResolvedValue(undefined),
    canvasMarkdown: vi.fn().mockResolvedValue('# The Plan\n\nhello\n'),
    exportCanvas: vi.fn().mockResolvedValue(undefined),
  } as unknown as AgentWorkspacesClient
}

async function mountPane(client: AgentWorkspacesClient, name: string | null = null) {
  const wrapper = mount(AgentCanvasPane, {
    props: { session: '7', workspace: 'web-app', name, client },
  })
  await flushPromises()
  return wrapper
}

describe('AgentCanvasPane', () => {
  beforeEach(() => {
    wailsEvents.handlers = []
    localStorage.clear()
    settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
    settingsBindings.SetCanvasFontSize.mockResolvedValue(undefined)
    settingsBindings.SetCanvasLineSpacing.mockResolvedValue(undefined)
    mermaid.render.mockReset().mockResolvedValue('<svg viewBox="0 0 100 40"><text>Request</text></svg>')
  })

  // Agent-authored markdown is untrusted: raw HTML must arrive escaped, never
  // as live elements in the webview.
  it('renders markdown with raw HTML escaped', async () => {
    const wrapper = await mountPane(
      fakeCanvasClient([
        block({ id: 'doc', body: '# Plan\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>' }),
      ]),
    )

    const body = wrapper.get('[data-testid="agent-canvas-block-doc"]')
    expect(body.find('script').exists()).toBe(false)
    expect(body.find('img').exists()).toBe(false)
    expect(body.text()).toContain('<script>alert(1)</script>')
    expect(body.find('h1').text()).toBe('Plan')
  })

  // An html block's body is sanitized on the Go read path, so the pane renders
  // it as markup under its own class scope — never through the markdown
  // renderer, which is shared with untrusted GitHub bodies and would escape it.
  it('renders an html block as live markup under the hv-html scope', async () => {
    const wrapper = await mountPane(
      fakeCanvasClient([
        block({
          id: 'stats',
          kind: 'html',
          title: 'Run',
          body: '<div class="hv-card hv-stat"><span class="hv-stat-value">42</span></div>',
        }),
      ]),
    )

    const rendered = wrapper.get('[data-testid="agent-canvas-block-stats"] .hv-html')
    expect(rendered.find('.hv-stat-value').text()).toBe('42')
    expect(rendered.classes()).not.toContain('markdown-body')
    expect(wrapper.get('[data-testid="agent-canvas-block-stats"] h2').text()).toBe('Run')
  })

  it('renders a fenced Mermaid diagram in markdown as a themed SVG', async () => {
    const wrapper = await mountPane(
      fakeCanvasClient([block({ id: 'flow', title: 'Request path', body: '```mermaid\nflowchart LR\nA --> B\n```' })]),
    )

    expect(mermaid.render).toHaveBeenCalledWith(
      'flowchart LR\nA --> B\n',
      expect.objectContaining({ darkMode: true, text: '#e9edf4', accent: '#f5b23f' }),
    )
    const diagram = wrapper.get('[data-testid="agent-canvas-block-flow-mermaid-0"]')
    const fittedViewBox = diagram.get('svg').attributes('viewBox')
    expect(diagram.get('[data-testid="agent-canvas-block-flow-mermaid-0-viewport"]').attributes('style')).toContain(
      'height: 220px',
    )
    expect(diagram.text()).toContain('Request')

    await diagram.get('[data-testid="agent-canvas-block-flow-mermaid-0-zoom-in"]').trigger('click')
    expect(diagram.get('[data-testid="agent-canvas-block-flow-mermaid-0-zoom"]').text()).toBe('125%')
    expect(diagram.get('svg').attributes('viewBox')).not.toBe(fittedViewBox)

    await diagram.get('[data-testid="agent-canvas-block-flow-mermaid-0-fit"]').trigger('click')
    expect(diagram.get('[data-testid="agent-canvas-block-flow-mermaid-0-zoom"]').text()).toBe('100%')
    expect(diagram.get('svg').attributes('viewBox')).toBe(fittedViewBox)
    expect(wrapper.get('[data-testid="agent-canvas-block-flow"] h2').text()).toBe('Request path')
  })

  it('shows a Mermaid parse error without breaking the surrounding markdown', async () => {
    mermaid.render.mockRejectedValueOnce(new Error('Parse error on line 2\nmore detail'))
    const wrapper = await mountPane(
      fakeCanvasClient([block({ id: 'flow', body: 'Before\n\n```mermaid\nnot a diagram\n```\n\nAfter' })]),
    )

    expect(wrapper.get('[data-testid="agent-canvas-block-flow-mermaid-1-error"]').text()).toBe(
      'Could not render Mermaid diagram: Parse error on line 2',
    )
    expect(wrapper.get('[data-testid="agent-canvas-block-flow"]').text()).toContain('Before')
    expect(wrapper.get('[data-testid="agent-canvas-block-flow"]').text()).toContain('After')
  })

  it('applies the global canvas reading settings to markdown and html without pane controls', async () => {
    settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'xl', canvasLineSpacing: 'relaxed' })
    const wrapper = await mountPane(
      fakeCanvasClient([
        block({ id: 'doc', body: '# Plan' }),
        block({ id: 'stats', kind: 'html', body: '<p>42 open</p>' }),
      ]),
    )

    for (const body of wrapper.findAll('.canvas-reading-body')) {
      const style = (body.element as HTMLElement).style
      expect(style.getPropertyValue('--hv-font-size')).toBe('18px')
      expect(style.getPropertyValue('--hv-line-height')).toBe('1.85')
    }
    expect(wrapper.find('[data-testid="agent-canvas-typography"]').exists()).toBe(false)
  })

  // The body here is what canvas.SanitizeHTML emits, viewBox spelled as SVG
  // needs it: the stylesheet scales a diagram by the aspect ratio that
  // attribute gives it, and this DOM does not case-correct it on re-parse.
  it("renders an html block's svg as real svg nodes", async () => {
    const wrapper = await mountPane(
      fakeCanvasClient([
        block({
          id: 'flow',
          kind: 'html',
          body: '<svg viewBox="0 0 200 60"><rect class="hv-node" x="1" y="1" width="70" height="34"/></svg>',
        }),
      ]),
    )

    const svg = wrapper.get('[data-testid="agent-canvas-block-flow"] .hv-html svg')
    expect(svg.element.namespaceURI).toBe('http://www.w3.org/2000/svg')
    expect(svg.element.getAttribute('viewBox')).toBe('0 0 200 60')
    expect(svg.get('rect').attributes('class')).toBe('hv-node')
  })

  it('intercepts links in an html block the same way as in markdown', async () => {
    const wrapper = await mountPane(
      fakeCanvasClient([
        block({ id: 'card', kind: 'html', body: '<p><a href="https://example.com/pr/1">the PR</a></p>' }),
      ]),
    )

    await wrapper.get('[data-testid="agent-canvas-block-card"] a').trigger('click')

    expect(wrapper.emitted('open-url')).toEqual([['https://example.com/pr/1']])
  })

  it('intercepts markdown links and emits open-url instead of navigating', async () => {
    const wrapper = await mountPane(
      fakeCanvasClient([block({ id: 'doc', body: '[the PR](https://example.com/pr/1)' })]),
    )

    await wrapper.get('[data-testid="agent-canvas-block-doc"] a').trigger('click')

    expect(wrapper.emitted('open-url')).toEqual([['https://example.com/pr/1']])
  })

  it('opens a link block through the same scheme filter', async () => {
    const wrapper = await mountPane(
      fakeCanvasClient([
        block({ id: 'ok', kind: 'link', title: 'The PR', url: 'https://example.com/pr/1' }),
        block({ id: 'bad', kind: 'link', title: 'Nope', url: 'javascript:alert(1)' }),
      ]),
    )

    await wrapper.get('[data-testid="agent-canvas-block-ok"] button').trigger('click')
    await wrapper.get('[data-testid="agent-canvas-block-bad"] button').trigger('click')

    expect(wrapper.emitted('open-url')).toEqual([['https://example.com/pr/1']])
  })

  // A relative link is a reference to another canvas in the workspace. It pins
  // that canvas through the route, the way a pick from the browse list does.
  it('emits pick for a link that names another canvas, and nothing for an unknown name', async () => {
    const wrapper = await mountPane(
      fakeCanvasClient(
        [block({ id: 'doc', body: '[the report](perf-report) and [a typo](perf-reprot)' })],
        [meta({ name: 'plan' }), meta({ name: 'perf-report' })],
      ),
    )

    const [known, unknown] = wrapper.findAll('[data-testid="agent-canvas-block-doc"] a')
    await known.trigger('click')
    await unknown.trigger('click')

    expect(wrapper.emitted('pick')).toEqual([['perf-report']])
    expect(wrapper.emitted('open-url')).toBeUndefined()
  })

  it('asks for the full-page view on the canvas it is showing', async () => {
    const wrapper = await mountPane(fakeCanvasClient([]), 'plan')

    await wrapper.get('[data-testid="agent-canvas-expand"]').trigger('click')

    expect(wrapper.emitted('open-page')).toEqual([['plan']])
  })

  // Hive wires a workspace's agent itself. A Code session's agent is the
  // user's to configure, so its empty pane says how.
  it('offers agent setup on an empty pane only where it is asked to', async () => {
    const inChats = await mountPane(fakeCanvasClient([], []))
    expect(inChats.find('[data-testid="agent-canvas-setup"]').exists()).toBe(false)

    const inCode = mount(AgentCanvasPane, {
      props: {
        session: 'abc123',
        workspace: 'acme/site',
        name: null,
        client: fakeCanvasClient([], []),
        agentSetup: true,
      },
    })
    await flushPromises()
    await inCode.get('[data-testid="agent-canvas-setup-open"]').trigger('click')
    expect(inCode.emitted('setup')).toHaveLength(1)

    const withCanvases = mount(AgentCanvasPane, {
      props: { session: 'abc123', workspace: 'acme/site', name: null, client: fakeCanvasClient([]), agentSetup: true },
    })
    await flushPromises()
    expect(withCanvases.find('[data-testid="agent-canvas-setup"]').exists()).toBe(false)
  })

  it('shows the empty state when the workspace has no canvases', async () => {
    const wrapper = await mountPane(fakeCanvasClient([], []))
    expect(wrapper.find('[data-testid="agent-canvas-empty"]').exists()).toBe(true)
  })

  // No route-pinned name: the default pick prefers the open chat's canvas
  // over a more recently updated one another chat made.
  it("defaults to the open chat's canvas", async () => {
    const client = fakeCanvasClient(
      [],
      [meta({ name: 'other-report', session: '9', updatedAt: 5 }), meta({ name: 'plan', session: '7', updatedAt: 3 })],
    )
    await mountPane(client)

    expect(vi.mocked(client.canvas)).toHaveBeenCalledWith('web-app', 'plan')
  })

  // In Code the pane's session is a hive session, and its canvases are the
  // repository's: the default prefers the one this session wrote.
  it("defaults to the hive session's own canvas among its repository's", async () => {
    const client = fakeCanvasClient(
      [],
      [
        meta({ workspace: 'acme/site', name: 'other', session: '', hiveSession: 'def456', updatedAt: 5 }),
        meta({ workspace: 'acme/site', name: 'plan', session: '', hiveSession: 'abc123', updatedAt: 3 }),
      ],
    )
    const wrapper = mount(AgentCanvasPane, {
      props: { session: 'abc123', workspace: 'acme/site', name: null, client },
    })
    await flushPromises()

    expect(vi.mocked(client.canvases)).toHaveBeenCalledWith('acme/site')
    expect(vi.mocked(client.canvas)).toHaveBeenCalledWith('acme/site', 'plan')

    wrapper.unmount()
  })

  it('pins the route-named canvas and lists canvases by title in the browse view', async () => {
    const client = fakeCanvasClient(
      [],
      [meta({ name: 'plan', title: 'The Plan', session: '7' }), meta({ name: 'perf-report', title: '', session: '9' })],
    )
    const wrapper = await mountPane(client, 'perf-report')

    expect(vi.mocked(client.canvas)).toHaveBeenCalledWith('web-app', 'perf-report')

    await wrapper.get('[data-testid="agent-canvas-title"]').trigger('click')
    const browse = wrapper.get('[data-testid="agent-canvas-browse"]')
    expect(browse.text()).toContain('The Plan')
    expect(browse.text()).toContain('perf-report')
    expect(wrapper.get('[data-testid="agent-canvas-browse-perf-report"]').attributes('aria-current')).toBe('true')
  })

  it('falls back to the default when the pinned canvas is deleted', async () => {
    const client = fakeCanvasClient(
      [],
      [meta({ name: 'plan', session: '7' }), meta({ name: 'perf-report', session: '9' })],
    )
    await mountPane(client, 'perf-report')

    vi.mocked(client.canvases).mockResolvedValue([meta({ name: 'plan', session: '7' })])
    wailsEvents.fire('canvas:updated', 9)
    await flushPromises()

    expect(vi.mocked(client.canvas)).toHaveBeenLastCalledWith('web-app', 'plan')
  })

  // open_canvas can name a canvas before the agent's first write to it.
  it('keeps a pinned name that was never listed', async () => {
    const client = fakeCanvasClient([], [meta({ name: 'plan' })])
    await mountPane(client, 'draft')

    wailsEvents.fire('canvas:updated', 7)
    await flushPromises()

    expect(vi.mocked(client.canvas)).toHaveBeenLastCalledWith('web-app', 'draft')
  })

  it('emits pick from the browse view instead of switching locally', async () => {
    const client = fakeCanvasClient(
      [],
      [meta({ name: 'plan', title: 'The Plan', session: '7' }), meta({ name: 'perf-report', session: '9' })],
    )
    const wrapper = await mountPane(client)

    await wrapper.get('[data-testid="agent-canvas-title"]').trigger('click')
    await wrapper.get('[data-testid="agent-canvas-browse-perf-report"]').trigger('click')

    expect(wrapper.emitted('pick')).toEqual([['perf-report']])
    expect(wrapper.find('[data-testid="agent-canvas-browse"]').exists()).toBe(false)
  })

  it('filters the browse list from the search input', async () => {
    const client = fakeCanvasClient([], [meta({ name: 'plan', title: 'The Plan' }), meta({ name: 'perf-report' })])
    const wrapper = await mountPane(client)

    await wrapper.get('[data-testid="agent-canvas-title"]').trigger('click')
    await wrapper.get('[data-testid="agent-canvas-search"]').setValue('perf')

    expect(wrapper.find('[data-testid="agent-canvas-browse-plan"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="agent-canvas-browse-perf-report"]').exists()).toBe(true)

    await wrapper.get('[data-testid="agent-canvas-search"]').setValue('nothing-matches')
    expect(wrapper.get('[data-testid="agent-canvas-browse"]').text()).toContain('No canvases match.')
  })

  it('groups the browse list by activity', async () => {
    const client = fakeCanvasClient(
      [],
      [meta({ name: 'fresh', updatedAt: Date.now() }), meta({ name: 'stale', updatedAt: 1 })],
    )
    const wrapper = await mountPane(client)

    await wrapper.get('[data-testid="agent-canvas-title"]').trigger('click')

    const browse = wrapper.get('[data-testid="agent-canvas-browse"]').text()
    expect(browse).toContain('Today')
    expect(browse).toContain('Older')
  })

  it('renders editable front matter with compact timestamp badges above the blocks', async () => {
    const wrapper = await mountPane(fakeCanvasClient([block({ id: 'doc', body: 'hello' })]))

    expect(wrapper.get('[data-testid="agent-canvas-frontmatter-tags"]').text()).toContain('release')
    expect(wrapper.get('[data-testid="agent-canvas-frontmatter-tags"]').text()).toContain('desktop')
    const created = wrapper.get('[data-testid="agent-canvas-frontmatter-created_at"]')
    const updated = wrapper.get('[data-testid="agent-canvas-frontmatter-updated_at"]')
    expect(created.element.tagName).toBe('SPAN')
    expect(updated.element.tagName).toBe('SPAN')
    expect(created.classes()).toContain('text-micro')
    expect(created.get('time').attributes('datetime')).toBe('1970-01-01T00:00:00.001Z')
    expect(updated.get('time').attributes('datetime')).toBe('1970-01-01T00:00:00.002Z')
  })

  it('copies the Go-rendered markdown to the native clipboard', async () => {
    const client = fakeCanvasClient([])
    const wrapper = await mountPane(client)

    await wrapper.get('[data-testid="agent-canvas-copy"]').trigger('click')
    await flushPromises()

    expect(vi.mocked(client.canvasMarkdown)).toHaveBeenCalledWith('web-app', 'plan')
    expect(runtime.setText).toHaveBeenCalledWith('# The Plan\n\nhello\n')
  })

  it('deletes after confirmation and selects the next canvas', async () => {
    const client = fakeCanvasClient([], [meta({ name: 'plan' }), meta({ name: 'report' })])
    const wrapper = await mountPane(client, 'plan')

    await wrapper.get('[data-testid="agent-canvas-delete"]').trigger('click')
    expect(vi.mocked(client.deleteCanvas)).not.toHaveBeenCalled()
    document.querySelector<HTMLButtonElement>('[data-testid="agent-canvas-delete-confirmation-confirm"]')?.click()
    await flushPromises()

    expect(vi.mocked(client.deleteCanvas)).toHaveBeenCalledWith('web-app', 'plan')
    expect(wrapper.emitted('pick')).toEqual([['report']])
  })

  it('saves through the native dialog and skips a cancelled one', async () => {
    const client = fakeCanvasClient([])
    const wrapper = await mountPane(client)

    await wrapper.get('[data-testid="agent-canvas-download"]').trigger('click')
    await flushPromises()
    expect(runtime.saveFile).toHaveBeenCalledWith(expect.objectContaining({ Filename: 'plan.md' }))
    expect(vi.mocked(client.exportCanvas)).toHaveBeenCalledWith('web-app', 'plan', '/tmp/plan.md')

    runtime.saveFile.mockResolvedValueOnce('')
    await wrapper.get('[data-testid="agent-canvas-download"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(client.exportCanvas)).toHaveBeenCalledTimes(1)
  })

  it('re-reads the canvas on canvas:updated', async () => {
    const client = fakeCanvasClient([])
    await mountPane(client)
    const readsBefore = vi.mocked(client.canvas).mock.calls.length

    wailsEvents.fire('canvas:updated', 7)
    await flushPromises()

    expect(vi.mocked(client.canvas).mock.calls.length).toBeGreaterThan(readsBefore)
  })
})
