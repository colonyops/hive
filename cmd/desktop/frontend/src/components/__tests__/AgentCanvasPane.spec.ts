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
    session: 7,
    hiveSession: '',
    createdAt: 1,
    updatedAt: 1,
    blockCount: 1,
    ...overrides,
  }
}

function fakeCanvasClient(blocks: CanvasBlock[], metas: WorkspaceCanvasMeta[] = [meta({})]) {
  return {
    canvas: vi
      .fn()
      .mockImplementation((workspace: string, name: string) =>
        Promise.resolve({ workspace, name, title: '', session: 7, createdAt: 1, updatedAt: 1, blocks }),
      ),
    canvases: vi.fn().mockResolvedValue(metas),
    canvasMarkdown: vi.fn().mockResolvedValue('# The Plan\n\nhello\n'),
    exportCanvas: vi.fn().mockResolvedValue(undefined),
  } as unknown as AgentWorkspacesClient
}

async function mountPane(client: AgentWorkspacesClient, name: string | null = null) {
  const wrapper = mount(AgentCanvasPane, {
    props: { session: 7, workspace: 'web-app', name, client },
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

  it('shows the empty state when the workspace has no canvases', async () => {
    const wrapper = await mountPane(fakeCanvasClient([], []))
    expect(wrapper.find('[data-testid="agent-canvas-empty"]').exists()).toBe(true)
  })

  // No route-pinned name: the default pick prefers the open chat's canvas
  // over a more recently updated one another chat made.
  it("defaults to the open chat's canvas", async () => {
    const client = fakeCanvasClient(
      [],
      [meta({ name: 'other-report', session: 9, updatedAt: 5 }), meta({ name: 'plan', session: 7, updatedAt: 3 })],
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
        meta({ workspace: 'acme/site', name: 'other', session: 0, hiveSession: 'def456', updatedAt: 5 }),
        meta({ workspace: 'acme/site', name: 'plan', session: 0, hiveSession: 'abc123', updatedAt: 3 }),
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
      [meta({ name: 'plan', title: 'The Plan', session: 7 }), meta({ name: 'perf-report', title: '', session: 9 })],
    )
    const wrapper = await mountPane(client, 'perf-report')

    expect(vi.mocked(client.canvas)).toHaveBeenCalledWith('web-app', 'perf-report')

    await wrapper.get('[data-testid="agent-canvas-title"]').trigger('click')
    const browse = wrapper.get('[data-testid="agent-canvas-browse"]')
    expect(browse.text()).toContain('The Plan')
    expect(browse.text()).toContain('perf-report')
    expect(wrapper.get('[data-testid="agent-canvas-browse-perf-report"]').attributes('aria-current')).toBe('true')
  })

  it('emits pick from the browse view instead of switching locally', async () => {
    const client = fakeCanvasClient(
      [],
      [meta({ name: 'plan', title: 'The Plan', session: 7 }), meta({ name: 'perf-report', session: 9 })],
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

  it('copies the Go-rendered markdown to the native clipboard', async () => {
    const client = fakeCanvasClient([])
    const wrapper = await mountPane(client)

    await wrapper.get('[data-testid="agent-canvas-copy"]').trigger('click')
    await flushPromises()

    expect(vi.mocked(client.canvasMarkdown)).toHaveBeenCalledWith('web-app', 'plan')
    expect(runtime.setText).toHaveBeenCalledWith('# The Plan\n\nhello\n')
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
