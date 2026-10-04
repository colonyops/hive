import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import AgentCanvasPage from '../AgentCanvasPage.vue'
import type { CanvasScope } from '../../lib/agentCanvas'
import type { CanvasBlock, WorkspaceCanvasMeta } from '../../lib/agentWorkspacesClient'
import { chooseOption } from '../../test-utils/select'

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

const agents = vi.hoisted(() => ({
  Available: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  workspaces: vi.fn(),
  canvas: vi.fn(),
  canvases: vi.fn(),
  canvasRepositories: vi.fn(),
  canvasMarkdown: vi.fn(),
  exportCanvas: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: agents.Available,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: agents.getAgentsEndpoint,
  createAgentWorkspacesClient: () => ({
    workspaces: agents.workspaces,
    canvas: agents.canvas,
    canvases: agents.canvases,
    canvasRepositories: agents.canvasRepositories,
    canvasMarkdown: agents.canvasMarkdown,
    exportCanvas: agents.exportCanvas,
  }),
}))

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

const listings: Record<string, WorkspaceCanvasMeta[]> = {
  'web-app': [meta({ name: 'plan', title: 'The Plan' }), meta({ name: 'perf-report', session: 9 })],
  docs: [meta({ workspace: 'docs', name: 'handbook', title: 'Handbook' })],
  'acme/site': [
    meta({ workspace: 'acme/site', name: 'runbook', session: 0, hiveSession: 'def456', updatedAt: 9 }),
    meta({ workspace: 'acme/site', name: 'handbook', session: 0, hiveSession: 'abc123', updatedAt: 2 }),
  ],
}
const bodies: Record<string, CanvasBlock[]> = {
  plan: [block({ id: 'doc', body: '# Plan\n\nSee [the report](perf-report) or [the PR](https://example.com/pr/1).' })],
  'perf-report': [block({ id: 'stats', kind: 'html', body: '<div class="hv-grid hv-cols-4"><p>412ms</p></div>' })],
  handbook: [block({ id: 'intro', body: 'Welcome.' })],
}

// The view is controlled: AppDialogs owns the scope and hands back each change.
async function mountPage(initial: Partial<CanvasScope> = {}) {
  const scope = ref<CanvasScope>({ workspace: 'web-app', name: null, session: 7, ...initial })
  const host = defineComponent({
    emits: ['close', 'open-url'],
    setup(_, { emit }) {
      return () =>
        h(AgentCanvasPage, {
          scope: scope.value,
          'onUpdate:scope': (next: CanvasScope) => (scope.value = next),
          onClose: () => emit('close'),
          onOpenUrl: (url: string) => emit('open-url', url),
        })
    },
  })
  const wrapper = mount(host, { attachTo: document.body })
  await flushPromises()
  return { wrapper, scope }
}

describe('AgentCanvasPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    wailsEvents.handlers = []
    localStorage.clear()
    document.body.innerHTML = ''
    settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
    agents.Available.mockResolvedValue({ available: true, reason: '' })
    agents.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
    agents.workspaces.mockResolvedValue({
      root: '',
      rootProblem: '',
      editor: { command: '', title: '' },
      presets: [],
      workspaces: [
        { dir: 'web-app', name: 'Web App' },
        { dir: 'docs', name: 'Docs' },
      ],
    })
    agents.canvasRepositories.mockResolvedValue(['acme/site'])
    agents.canvases.mockImplementation((workspace: string) => Promise.resolve(listings[workspace] ?? []))
    agents.canvas.mockImplementation((workspace: string, name: string) =>
      Promise.resolve({
        workspace,
        name,
        title: '',
        session: 7,
        createdAt: 1,
        updatedAt: 1,
        blocks: bodies[name] ?? [],
      }),
    )
    agents.canvasMarkdown.mockResolvedValue('# The Plan\n')
    agents.exportCanvas.mockResolvedValue(undefined)
  })

  it("lists the workspace's canvases beside the one it is showing", async () => {
    const { wrapper } = await mountPage()

    expect(agents.canvas).toHaveBeenCalledWith('web-app', 'plan')
    expect(wrapper.get('[data-testid="canvas-page-block-doc"] h1').text()).toBe('Plan')
    expect(wrapper.get('[data-testid="canvas-page-browse-plan"]').attributes('aria-current')).toBe('true')
    expect(wrapper.get('[data-testid="canvas-page-browse-perf-report"]').attributes('aria-current')).toBeUndefined()

    wrapper.unmount()
  })

  it('opens on the canvas it was asked for', async () => {
    const { wrapper } = await mountPage({ name: 'perf-report' })

    expect(agents.canvas).toHaveBeenCalledWith('web-app', 'perf-report')
    expect(wrapper.find('[data-testid="canvas-page-block-stats"] .hv-html .hv-cols-4').exists()).toBe(true)

    wrapper.unmount()
  })

  it('moves to another canvas from the sidebar without leaving the view', async () => {
    const { wrapper, scope } = await mountPage()

    await wrapper.get('[data-testid="canvas-page-browse-perf-report"]').trigger('click')
    await flushPromises()

    expect(scope.value).toEqual({ workspace: 'web-app', name: 'perf-report', session: 7 })
    expect(wrapper.find('[data-testid="canvas-page-block-stats"]').exists()).toBe(true)
    expect(wrapper.emitted('close')).toBeUndefined()

    wrapper.unmount()
  })

  it('opens a link to another canvas in place and a web link outside the app', async () => {
    const { wrapper, scope } = await mountPage()

    const [toCanvas, toWeb] = wrapper.findAll('[data-testid="canvas-page-block-doc"] a')
    await toWeb.trigger('click')
    expect(wrapper.emitted('open-url')).toEqual([['https://example.com/pr/1']])
    expect(scope.value.name).toBeNull()

    await toCanvas.trigger('click')
    await flushPromises()
    expect(scope.value.name).toBe('perf-report')
    expect(wrapper.find('[data-testid="canvas-page-block-stats"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('filters the sidebar from the search field', async () => {
    const { wrapper } = await mountPage()

    await wrapper.get('[data-testid="canvas-page-search"]').setValue('perf')

    expect(wrapper.find('[data-testid="canvas-page-browse-plan"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="canvas-page-browse-perf-report"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('copies and saves the canvas on screen as Markdown', async () => {
    const { wrapper } = await mountPage()

    await wrapper.get('[data-testid="canvas-page-copy"]').trigger('click')
    await flushPromises()
    expect(agents.canvasMarkdown).toHaveBeenCalledWith('web-app', 'plan')
    expect(runtime.setText).toHaveBeenCalledWith('# The Plan\n')

    await wrapper.get('[data-testid="canvas-page-download"]').trigger('click')
    await flushPromises()
    expect(runtime.saveFile).toHaveBeenCalledWith(expect.objectContaining({ Filename: 'plan.md' }))
    expect(agents.exportCanvas).toHaveBeenCalledWith('web-app', 'plan', '/tmp/plan.md')

    wrapper.unmount()
  })

  it('switches workspace from the sidebar and lands on its most recent canvas', async () => {
    const { wrapper, scope } = await mountPage()

    await chooseOption(wrapper, 'canvas-page-workspace', 'docs')
    await flushPromises()

    expect(scope.value).toEqual({ workspace: 'docs', name: null, session: null })
    expect(agents.canvas).toHaveBeenLastCalledWith('docs', 'handbook')

    wrapper.unmount()
  })

  it("reads a repository's canvases when it is opened from a Code session", async () => {
    const { wrapper } = await mountPage({ workspace: 'acme/site', session: 'abc123' })

    expect(agents.canvases).toHaveBeenCalledWith('acme/site')
    expect(agents.canvas).toHaveBeenCalledWith('acme/site', 'handbook')
    expect(wrapper.get('[data-testid="canvas-page-workspace"]').text()).toContain('acme/site')

    wrapper.unmount()
  })

  it('offers agent setup for a repository with no canvases, and not for a workspace', async () => {
    const repo = await mountPage({ workspace: 'acme/empty', session: 'abc123' })
    const setup = repo.wrapper.findComponent(AgentCanvasPage)
    await repo.wrapper.get('[data-testid="canvas-page-setup-open"]').trigger('click')
    expect(setup.emitted('setup')).toHaveLength(1)
    repo.wrapper.unmount()

    agents.canvases.mockResolvedValue([])
    const workspace = await mountPage({ workspace: 'web-app' })
    expect(workspace.wrapper.find('[data-testid="canvas-page-setup"]').exists()).toBe(false)
    workspace.wrapper.unmount()
  })

  it('offers the repositories that hold a canvas beside the workspaces', async () => {
    const { wrapper, scope } = await mountPage()

    await chooseOption(wrapper, 'canvas-page-workspace', 'acme/site')
    await flushPromises()

    expect(scope.value).toEqual({ workspace: 'acme/site', name: null, session: null })
    expect(agents.canvas).toHaveBeenLastCalledWith('acme/site', 'runbook')

    wrapper.unmount()
  })

  it('falls back to the first workspace when it is opened with none', async () => {
    const { wrapper } = await mountPage({ workspace: '', session: null })

    expect(agents.canvases).toHaveBeenCalledWith('web-app')
    expect(wrapper.find('[data-testid="canvas-page-block-doc"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('re-reads on canvas:updated', async () => {
    const { wrapper } = await mountPage()
    const readsBefore = agents.canvas.mock.calls.length

    wailsEvents.fire('canvas:updated', 7)
    await flushPromises()

    expect(agents.canvas.mock.calls.length).toBeGreaterThan(readsBefore)

    wrapper.unmount()
  })

  it('closes from its button and on Escape', async () => {
    const { wrapper } = await mountPage()

    await wrapper.get('[data-testid="canvas-page-close"]').trigger('click')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(2)

    wrapper.unmount()
  })

  it('says why when this build has no Chats area', async () => {
    agents.Available.mockResolvedValue({ available: false, reason: 'no ptyterm on this build.' })
    const { wrapper } = await mountPage()

    expect(wrapper.get('[data-testid="canvas-page-unavailable"]').text()).toContain('no ptyterm on this build.')
    expect(wrapper.find('[data-testid="canvas-page-sidebar"]').exists()).toBe(false)

    wrapper.unmount()
  })
})
