import { createMemoryHistory, type Router } from 'vue-router'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentsMode from '../AgentsMode.vue'
import AgentsSidebar from '../AgentsSidebar.vue'
import NewChatDialog from '../NewChatDialog.vue'
import { resetAgentWorkspacesForTests } from '../../composables/useAgentWorkspaces'
import { resetAgentSessionsAllForTests } from '../../composables/useAgentSessionsAll'
import { createAppRouter } from '../../router'
import { tooltipFor } from '../../test-utils/tooltip'
import type { MissingSkillPackage } from '../../lib/agentWorkspacesClient'

// App.vue mounts AgentsMode once and hides it with v-show on a trip to the
// hub (ADR terminal-mode-is-hidden-not-unmounted): the component itself must never re-key or v-if anything
// internal to props.active, or a v-show'd parent would still pay for a
// rebuild on every round trip. This asserts that at the component level —
// App.spec.ts separately proves the parent uses v-show rather than v-if.

const xterm = vi.hoisted(() => {
  class FakeTerminal {
    static instances: FakeTerminal[] = []
    cols = 80
    rows = 24
    options: Record<string, unknown>
    write = vi.fn()
    open = vi.fn()
    loadAddon = vi.fn()
    dispose = vi.fn()
    resize = vi.fn()
    focus = vi.fn()
    scrollToBottom = vi.fn()
    parser = {
      registerCsiHandler: vi.fn(() => ({ dispose: vi.fn() })),
      registerDcsHandler: vi.fn(() => ({ dispose: vi.fn() })),
      registerOscHandler: vi.fn(() => ({ dispose: vi.fn() })),
    }

    // Enough of xterm's buffer for watchTailPin: a mutable viewport position
    // and the two events that can move it.
    buffer = {
      active: { baseY: 0, viewportY: 0 },
      onBufferChange: (handler: () => void) => {
        this.bufferChangeHandler = handler
        return { dispose: vi.fn() }
      },
    }

    dataHandler: ((data: string) => void) | null = null
    scrollHandler: (() => void) | null = null
    bufferChangeHandler: (() => void) | null = null

    constructor(options: Record<string, unknown> = {}) {
      this.options = { ...options }
      FakeTerminal.instances.push(this)
    }

    onData(handler: (data: string) => void) {
      this.dataHandler = handler
      return { dispose: vi.fn() }
    }

    onScroll(handler: () => void) {
      this.scrollHandler = handler
      return { dispose: vi.fn() }
    }
  }

  class FakeAddon {
    dispose = vi.fn()
    activate = vi.fn()
    fit = vi.fn()
    proposeDimensions = vi.fn(() => ({ cols: 132, rows: 43 }))
  }

  return { FakeTerminal, FakeAddon }
})

vi.mock('@xterm/xterm', () => ({ Terminal: xterm.FakeTerminal }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: xterm.FakeAddon }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: xterm.FakeAddon }))

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  Endpoint: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  createAgentWorkspacesClient: vi.fn(),
  loadTerminalFaces: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
  Endpoint: mocks.Endpoint,
}))
vi.mock('../../lib/terminalFaces', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/terminalFaces')>()),
  loadTerminalFaces: mocks.loadTerminalFaces,
}))
vi.mock('../../lib/terminalRenderer', () => ({ claimAtlasRenderer: vi.fn() }))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: mocks.getAgentsEndpoint,
  createAgentWorkspacesClient: mocks.createAgentWorkspacesClient,
}))

// Captures every useWailsEvent registration (AgentsMode's dot and the canvas
// pane's refetch both listen) so a test can fire canvas:updated by hand.
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

class FakeSocket {
  static OPEN = 1
  readyState = 1
  binaryType = 'blob'
  onmessage: ((event: { data: ArrayBuffer }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  sent: Uint8Array[] = []
  send(frame: Uint8Array): void { this.sent.push(frame) }
  close(): void {}
}

// Mirrors the window-event frame in internal/adapter/httpapi/terminal_stream.go.
function windowFrame(state: Record<string, unknown>): ArrayBuffer {
  const body = new TextEncoder().encode(JSON.stringify(state))
  const bytes = new Uint8Array(1 + body.length)
  bytes[0] = 0x01
  bytes.set(body, 1)
  return bytes.buffer
}

function openedSocket(client: ReturnType<typeof fakeClient>): FakeSocket {
  return client.openStream.mock.results.at(-1)!.value as FakeSocket
}

function typeInPane(data: string): void {
  xterm.FakeTerminal.instances.at(-1)!.dataHandler?.(data)
}

function inputFrame(paneId: string, data: string): number[] {
  const id = Array.from(new TextEncoder().encode(paneId))
  return [0x10, id.length, ...id, ...Array.from(new TextEncoder().encode(data))]
}

class FakeResizeObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  constructor(_callback: () => void) {}
}

const weeklySummary = {
  id: 'weekly-summary', name: 'Weekly summary', cron: '0 9 * * 5',
  prompt: 'Summarize the week.', disabled: false, onMissed: 'run' as const,
  nextRunAt: null, lastRun: null,
}

const workspaceRows = [
  { dir: 'web-app', name: 'Web App', command: 'claude', danger: false, mcps: [], skills: [], schedules: [weeklySummary], problem: '', notice: '' },
  { dir: 'api', name: 'API', command: 'claude', danger: false, mcps: [], skills: [], schedules: [], problem: '', notice: '' },
]

// A chat row as the cross-workspace listing reports it: terminalId set means
// the listing's tmux probe found the session alive.
const chatRow = {
  id: 7, workspace: 'web-app', name: 'New Chat', agent: 'claude', lastOpenedAt: 0,
  terminalId: 'agentws-7', windowId: '', paneId: '', cols: 0, rows: 0, resumeAttempted: false, notice: '',
  scheduleId: '',
}

function fakeClient(editor = { command: 'zed', title: 'Zed' }) {
  return {
    workspaces: vi.fn().mockResolvedValue({
      root: '/tmp/agents', rootProblem: '', available: true, error: '',
      workspaces: workspaceRows, agents: ['claude'], presets: [], editor,
    }),
    openWorkspace: vi.fn((dir: string) => Promise.resolve({
      workspace: workspaceRows.find((ws) => ws.dir === dir) ?? workspaceRows[0],
      sessions: [], missingMcps: [], missingPackages: [] as MissingSkillPackage[],
    })),
    allSessions: vi.fn().mockResolvedValue([]),
    activity: vi.fn().mockResolvedValue([]),
    startSession: vi.fn().mockResolvedValue({
      id: 7, workspace: 'web-app', name: 'New Chat', agent: 'claude', lastOpenedAt: 0,
      terminalId: 't1', windowId: 'w1', paneId: '%1', cols: 80, rows: 24, resumeAttempted: false, notice: '',
    }),
    resumeSession: vi.fn().mockResolvedValue({ ...chatRow, windowId: 'w1', cols: 80, rows: 24, resumeAttempted: true }),
    closeSession: vi.fn().mockResolvedValue({ closed: true }),
    resizeSession: vi.fn().mockResolvedValue(undefined),
    openStream: vi.fn(() => new FakeSocket()),
    openWorkspaceInEditor: vi.fn().mockResolvedValue(undefined),
    revealWorkspace: vi.fn().mockResolvedValue(undefined),
    mcpCatalogue: vi.fn().mockResolvedValue([]),
    skillPackages: vi.fn().mockResolvedValue({ packages: [], skills: [], problem: '' }),
    revealSkillPackages: vi.fn().mockResolvedValue(undefined),
    revealSharedSkills: vi.fn().mockResolvedValue(undefined),
    canvas: vi.fn().mockResolvedValue({ workspace: 'web-app', name: 'plan', title: '', session: 7, createdAt: 0, updatedAt: 0, blocks: [] }),
    canvases: vi.fn().mockResolvedValue([]),
    scheduleRuns: vi.fn().mockResolvedValue([]),
    runSchedule: vi.fn(),
    previewSchedule: vi.fn().mockResolvedValue({ next: [], cronError: '', promptError: '' }),
  }
}

async function mountAgentsMode(path = '/workspaces') {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()
  const wrapper = mount(AgentsMode, { global: { plugins: [router] }, props: { active: true } })
  await flushPromises()
  return { wrapper, router }
}

// Starting a chat is the sidebar's + followed by the dialog it opens; the
// dialog teleports, so the mode-level tests drive it through its component.
async function startChat(wrapper: VueWrapper, workspace = 'web-app', name = '') {
  wrapper.findComponent(AgentsSidebar).vm.$emit('request-new-session')
  await flushPromises()
  wrapper.findComponent(NewChatDialog).vm.$emit('submit', { workspace, name })
  await flushPromises()
}

// The canvas toggle lives in the title bar (#432), which these mode-level tests
// do not mount, so they drive ?canvas directly — the route is the source of
// truth either way.
async function setCanvas(router: Router, value: string | undefined) {
  await router.replace({ query: { ...router.currentRoute.value.query, canvas: value } })
  await flushPromises()
}

// The status bar only exists over a launching or live pane, so every bar test
// starts a chat first.
async function mountWithOpenChat(client = fakeClient()) {
  mocks.createAgentWorkspacesClient.mockReturnValue(client)
  const { wrapper, router } = await mountAgentsMode('/workspaces/web-app')
  await startChat(wrapper)
  return { wrapper, router, client }
}

function setHidden(hidden: boolean): void {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => (hidden ? 'hidden' : 'visible') })
  document.dispatchEvent(new Event('visibilitychange'))
}

describe('AgentsMode', () => {
  beforeEach(() => {
    resetAgentWorkspacesForTests()
    resetAgentSessionsAllForTests()
    xterm.FakeTerminal.instances = []
    wailsEvents.handlers = []
    globalThis.ResizeObserver = FakeResizeObserver as unknown as typeof ResizeObserver
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.Endpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://127.0.0.1:1/s', token: 'test' })
    mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://127.0.0.1:1/s', token: 'test' })
    mocks.createAgentWorkspacesClient.mockReturnValue(fakeClient())
    mocks.loadTerminalFaces.mockResolvedValue(undefined)
  })

  // #307: a manifest written before packages enumerates skill slugs, and
  // "Missing skill packages: hive-flows" reads as a failed install of the
  // skills that were in fact installed — by the package sitting in the same
  // list. The banner has to name what the entry is and which package carries
  // it, and keep a genuine typo as its own separate report.
  it('separates an enabled name that is a skill from one that matches nothing', async () => {
    const client = fakeClient()
    client.openWorkspace = vi.fn((_dir: string) => Promise.resolve({
      workspace: workspaceRows[0],
      sessions: [],
      missingMcps: [],
      missingPackages: [
        { name: 'hive-flows', skill: true, selectedBy: ['hive'] },
        { name: 'ghost', skill: false, selectedBy: [] },
      ] as MissingSkillPackage[],
    }))
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    const { wrapper } = await mountAgentsMode('/workspaces/web-app')

    const skills = wrapper.find('[data-testid="agents-missing-skills"]')
    expect(skills.exists()).toBe(true)
    expect(skills.text()).toContain('hive-flows is a skill, not a package')
    expect(skills.text()).toContain('the hive package selects it')

    const packages = wrapper.find('[data-testid="agents-missing-packages"]')
    expect(packages.exists()).toBe(true)
    expect(packages.text()).toBe('Missing skill packages: ghost')
  })

  it('parks the activity poll while the window is hidden and polls again when it shows', async () => {
    const client = fakeClient()
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await mountAgentsMode()
      await vi.advanceTimersByTimeAsync(2000)
      const polled = client.activity.mock.calls.length
      expect(polled).toBeGreaterThan(0)

      setHidden(true)
      await flushPromises()
      await vi.advanceTimersByTimeAsync(10_000)
      expect(client.activity).toHaveBeenCalledTimes(polled)

      setHidden(false)
      await flushPromises()
      expect(client.activity).toHaveBeenCalledTimes(polled + 1)
    } finally {
      vi.useRealTimers()
      setHidden(false)
    }
  })

  it('keeps the same root and sidebar elements across an active toggle', async () => {
    const { wrapper } = await mountAgentsMode()

    const rootBefore = wrapper.find('[data-testid="agents-mode"]').element
    const sidebarBefore = wrapper.find('[data-testid="agents-workspace-sidebar"]').element

    await wrapper.setProps({ active: false })
    await flushPromises()
    await wrapper.setProps({ active: true })
    await flushPromises()

    expect(wrapper.find('[data-testid="agents-mode"]').element).toBe(rootBefore)
    expect(wrapper.find('[data-testid="agents-workspace-sidebar"]').element).toBe(sidebarBefore)
  })

  it('drops the sidebar on the collapse prop and rebuilds it on the way back', async () => {
    const { wrapper } = await mountAgentsMode()
    expect(wrapper.find('[data-testid="agents-workspace-sidebar"]').exists()).toBe(true)

    await wrapper.setProps({ sidebarCollapsed: true })
    await flushPromises()
    expect(wrapper.find('[data-testid="agents-workspace-sidebar"]').exists()).toBe(false)

    await wrapper.setProps({ sidebarCollapsed: false })
    await flushPromises()
    expect(wrapper.find('[data-testid="agents-workspace-sidebar"]').exists()).toBe(true)
  })

  it('reports the unavailable reason and offers a retry when ptyterm is unavailable', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'agent workspaces need macOS or Linux and a desktop build.' })
    const { wrapper } = await mountAgentsMode()

    expect(wrapper.find('[data-testid="agents-unavailable"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="agents-unavailable-reason"]').text())
      .toBe('agent workspaces need macOS or Linux and a desktop build.')
    expect(wrapper.find('[data-testid="agents-workspace-sidebar"]').exists()).toBe(false)
  })

  it('raises the status bar over an open chat and opens its workspace outside the app', async () => {
    mocks.createAgentWorkspacesClient.mockReturnValue(fakeClient())
    const { wrapper } = await mountAgentsMode('/workspaces/web-app')
    expect(wrapper.find('[data-testid="agents-pane-statusbar"]').exists()).toBe(false)

    await startChat(wrapper)

    expect(wrapper.get('[data-testid="agents-pane-statusbar-workspace"]').text()).toBe('Web App')
    expect(tooltipFor(wrapper, 'agents-pane-statusbar-open-editor')).toBe('Open in Zed')
  })

  it('routes the bar actions at the open chat\'s workspace directory', async () => {
    const { wrapper, client } = await mountWithOpenChat()

    await wrapper.get('[data-testid="agents-pane-statusbar-open-editor"]').trigger('click')
    await wrapper.get('[data-testid="agents-pane-statusbar-reveal"]').trigger('click')

    expect(client.openWorkspaceInEditor).toHaveBeenCalledWith('web-app')
    expect(client.revealWorkspace).toHaveBeenCalledWith('web-app')
  })

  // The open chat need not belong to the focused workspace: moving the focus
  // filter must not repoint the bar (or its actions) at the newly focused one.
  it('keeps naming the open chat\'s workspace when the focus filter moves', async () => {
    const { wrapper, router } = await mountWithOpenChat()

    await router.push({ name: 'agents', params: { workspace: 'api' } })
    await flushPromises()

    expect(wrapper.get('[data-testid="agents-pane-statusbar-workspace"]').text()).toBe('Web App')
  })

  it('offers no editor action when no editor is configured', async () => {
    const { wrapper } = await mountWithOpenChat(fakeClient({ command: '', title: '' }))

    expect(wrapper.find('[data-testid="agents-pane-statusbar-open-editor"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="agents-pane-statusbar-reveal"]').exists()).toBe(true)
  })

  it('names the open chat in the route and clears it when the chat closes', async () => {
    const { wrapper, router, client } = await mountWithOpenChat()
    expect(router.currentRoute.value.query.chat).toBe('7')

    wrapper.findComponent(AgentsSidebar).vm.$emit('close-session', { ...chatRow })
    await flushPromises()

    expect(client.closeSession).toHaveBeenCalledWith(7)
    expect(router.currentRoute.value.query.chat).toBeUndefined()
  })

  it('carries the open chat across a focus switch', async () => {
    const { wrapper, router } = await mountWithOpenChat()

    wrapper.findComponent(AgentsSidebar).vm.$emit('select-workspace', 'api')
    await flushPromises()

    expect(router.currentRoute.value.params.workspace).toBe('api')
    expect(router.currentRoute.value.query.chat).toBe('7')
  })

  // The reload path: the hash preserved the route, the pane starts idle, and
  // the routed chat's tmux session is still running — so it reattaches.
  it('reattaches the routed chat when its tmux session is live', async () => {
    const client = fakeClient()
    client.allSessions.mockResolvedValue([chatRow])
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    const { wrapper } = await mountAgentsMode('/workspaces/web-app?chat=7')

    expect(client.resumeSession).toHaveBeenCalledWith({ id: 7 })
    expect(wrapper.get('[data-testid="agents-pane-statusbar-workspace"]').text()).toBe('Web App')
  })

  // ResumeSession relaunches a dead session, and a relaunch must stay a
  // deliberate click — the routed chat is dropped, never resumed blind.
  it('drops the routed chat instead of relaunching it when its session is dead', async () => {
    const client = fakeClient()
    client.allSessions.mockResolvedValue([{ ...chatRow, terminalId: '' }])
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    const { router } = await mountAgentsMode('/workspaces/web-app?chat=7')

    expect(client.resumeSession).not.toHaveBeenCalled()
    expect(router.currentRoute.value.query.chat).toBeUndefined()
  })

  // A route naming another chat is a request to switch, whatever the pane is
  // holding: the live pane is torn down and the named chat attached.
  it('switches the live pane to another routed chat when that one is live', async () => {
    const client = fakeClient()
    const other = { ...chatRow, id: 9, name: 'Second', terminalId: 'agentws-9' }
    client.allSessions.mockResolvedValue([other])
    client.resumeSession.mockResolvedValue({ ...other, windowId: 'w9', cols: 80, rows: 24, resumeAttempted: true })
    const { router } = await mountWithOpenChat(client)

    await router.push({ name: 'agents', params: { workspace: 'web-app' }, query: { chat: '9', canvas: '1' } })
    await flushPromises()

    expect(client.resumeSession).toHaveBeenCalledWith({ id: 9 })
    expect(router.currentRoute.value.query.chat).toBe('9')
  })

  // A route naming a chat that cannot be attached leaves the live one alone:
  // clearing ?chat here would drop ?canvas too and unmount the canvas pane of
  // a chat that is still running.
  it('keeps the live chat in the route when the routed chat is dead', async () => {
    const client = fakeClient()
    client.allSessions.mockResolvedValue([{ ...chatRow, id: 9, name: 'Second', terminalId: '' }])
    const { router } = await mountWithOpenChat(client)

    await router.push({ name: 'agents', params: { workspace: 'web-app' }, query: { chat: '9', canvas: '1' } })
    await flushPromises()

    expect(client.resumeSession).not.toHaveBeenCalled()
    expect(router.currentRoute.value.query.chat).toBe('7')
    expect(router.currentRoute.value.query.canvas).toBe('1')
  })

  // Clicking the row already open in the pane must not tear it down and
  // re-resume it (#433).
  it('does nothing when the sidebar selects the session already open in the pane', async () => {
    const { wrapper, client } = await mountWithOpenChat()

    wrapper.findComponent(AgentsSidebar).vm.$emit('select-session', { ...chatRow })
    await flushPromises()

    expect(client.resumeSession).not.toHaveBeenCalled()
  })

  // A launch that lands while an editable field holds focus must not steal it
  // -- the sidebar's inline rename opens on a double click whose first click
  // is what started this very launch (#434 follow-up).
  it('does not steal focus from an editable field a launch resolves under', async () => {
    const client = fakeClient()
    const other = { ...chatRow, id: 9, name: 'Second', terminalId: 'agentws-9' }
    let resolveResume: ((session: typeof other) => void) | undefined
    client.resumeSession.mockImplementation(() => new Promise((resolve) => { resolveResume = resolve }))
    const { wrapper } = await mountWithOpenChat(client)

    const field = document.createElement('input')
    document.body.appendChild(field)
    field.focus()
    expect(document.activeElement).toBe(field)

    wrapper.findComponent(AgentsSidebar).vm.$emit('select-session', other)
    await flushPromises()
    resolveResume?.({ ...other, windowId: 'w9', cols: 80, rows: 24, resumeAttempted: true })
    await flushPromises()

    const created = xterm.FakeTerminal.instances.at(-1)!
    expect(created.focus).not.toHaveBeenCalled()
    field.remove()
  })

  it('focuses the newly attached pane when nothing editable holds focus', async () => {
    const client = fakeClient()
    const other = { ...chatRow, id: 9, name: 'Second', terminalId: 'agentws-9' }
    client.resumeSession.mockResolvedValue({ ...other, windowId: 'w9', cols: 80, rows: 24, resumeAttempted: true })
    const { wrapper } = await mountWithOpenChat(client)

    wrapper.findComponent(AgentsSidebar).vm.$emit('select-session', other)
    await flushPromises()

    const created = xterm.FakeTerminal.instances.at(-1)!
    expect(created.focus).toHaveBeenCalled()
  })

  // Parity with the Code view's pill (#469): a chat that keeps streaming while
  // the viewport is scrolled up has to offer the way back.
  it('offers a way back to the live tail while the chat pane is scrolled up', async () => {
    const { wrapper } = await mountWithOpenChat()
    expect(wrapper.find('[data-testid="agents-scroll-to-bottom"]').exists()).toBe(false)

    const created = xterm.FakeTerminal.instances.at(-1)!
    created.buffer.active.baseY = 200
    created.buffer.active.viewportY = 100
    created.scrollHandler?.()
    await flushPromises()

    await wrapper.get('[data-testid="agents-scroll-to-bottom"]').trigger('click')
    expect(created.scrollToBottom).toHaveBeenCalled()

    created.buffer.active.viewportY = 200
    created.scrollHandler?.()
    await flushPromises()
    expect(wrapper.find('[data-testid="agents-scroll-to-bottom"]').exists()).toBe(false)
  })

  it('leaves the pill alone for a nudge inside the tail slack', async () => {
    const { wrapper } = await mountWithOpenChat()

    const created = xterm.FakeTerminal.instances.at(-1)!
    created.buffer.active.baseY = 200
    created.buffer.active.viewportY = 196
    created.scrollHandler?.()
    await flushPromises()

    expect(wrapper.find('[data-testid="agents-scroll-to-bottom"]').exists()).toBe(false)
  })

  // The pane is torn down on the way out, so a chat reopened at the tail must
  // not inherit the last one's pill.
  it('clears the pill when the pane goes back to the zero state', async () => {
    const { wrapper, client } = await mountWithOpenChat()

    const created = xterm.FakeTerminal.instances.at(-1)!
    created.buffer.active.baseY = 200
    created.buffer.active.viewportY = 100
    created.scrollHandler?.()
    await flushPromises()
    expect(wrapper.find('[data-testid="agents-scroll-to-bottom"]').exists()).toBe(true)

    openedSocket(client).onclose?.()
    await flushPromises()

    expect(wrapper.find('[data-testid="agents-scroll-to-bottom"]').exists()).toBe(false)
  })

  it('frames keystrokes for the pane the launch named', async () => {
    const { client } = await mountWithOpenChat()
    const socket = openedSocket(client)

    typeInPane('x')

    expect(socket.sent.map((frame) => Array.from(frame))).toEqual([[0x10, 2, 0x25, 0x31, 0x78]])
  })

  // Stream window events override a stale active pane from the launch response.
  it('re-points input at the active pane a window event names', async () => {
    const { client } = await mountWithOpenChat()
    const socket = openedSocket(client)

    socket.onmessage?.({ data: windowFrame({ kind: 'active-changed', windowId: 'w1', activePane: '%5', width: 80, height: 24 }) })
    typeInPane('x')

    expect(Array.from(socket.sent.at(-1)!)).toEqual(inputFrame('%5', 'x'))
  })

  // Resume returns no pane ID, so input waits for the stream to name one.
  it('drops input on a resumed chat until a window event names its pane', async () => {
    const client = fakeClient()
    client.allSessions.mockResolvedValue([chatRow])
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    await mountAgentsMode('/workspaces/web-app?chat=7')
    const socket = openedSocket(client)

    typeInPane('x')
    expect(socket.sent).toEqual([])

    socket.onmessage?.({ data: windowFrame({ kind: 'layout-changed', windowId: 'w1', activePane: '%3', width: 80, height: 24 }) })
    typeInPane('x')
    expect(socket.sent.map((frame) => Array.from(frame))).toEqual([inputFrame('%3', 'x')])
  })

  // A failed attach sets openSessionId optimistically but leaves paneStatus
  // 'idle', so the guard must key on the pair — openSessionId alone would
  // make a retry click on the same row a no-op too.
  it('still resumes the session on a retry click after a failed attach left it idle', async () => {
    const client = fakeClient()
    client.startSession.mockResolvedValue({
      id: 7, workspace: 'web-app', name: 'New Chat', agent: 'claude', lastOpenedAt: 0,
      terminalId: '', windowId: '', paneId: '', cols: 0, rows: 0, resumeAttempted: false, notice: 'boom',
    })
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    const { wrapper } = await mountAgentsMode('/workspaces/web-app')
    await startChat(wrapper)

    wrapper.findComponent(AgentsSidebar).vm.$emit('select-session', { ...chatRow })
    await flushPromises()

    expect(client.resumeSession).toHaveBeenCalledWith({ id: 7 })
  })

  // A schedule firing at 09:00 starts a chat nobody clicked for: the tree
  // learns about it from the wake-up, not from the next user action.
  it('reloads the chat list when a schedule reports a change', async () => {
    const { client } = await mountWithOpenChat()
    const before = client.allSessions.mock.calls.length

    wailsEvents.fire('schedules:updated', 'web-app')
    await flushPromises()

    expect(client.allSessions.mock.calls.length).toBeGreaterThan(before)
  })

  it('reports a failed open/reveal in the bar instead of dropping it', async () => {
    const { wrapper, client } = await mountWithOpenChat()
    client.revealWorkspace.mockRejectedValue(new Error('the directory is gone'))

    await wrapper.get('[data-testid="agents-pane-statusbar-reveal"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="agents-pane-statusbar-error"]').text()).toBe('the directory is gone')
  })

  // A session that dies before the attach answers 200 with no terminalId and
  // the reason in its notice. The listing carries no notice, so this response
  // is the only place it exists — dropping it made a failed launch look like a
  // click that did nothing.
  it('reports the launch notice when a session exits before it can be attached', async () => {
    const client = fakeClient()
    client.startSession.mockResolvedValue({
      id: 7, workspace: 'web-app', name: 'New Chat', agent: 'claude', lastOpenedAt: 0,
      terminalId: '', windowId: '', paneId: '', cols: 0, rows: 0, resumeAttempted: true,
      notice: 'the session exited immediately; check that the agent CLI is installed and on PATH',
    })
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    const { wrapper } = await mountAgentsMode('/workspaces/web-app')

    await startChat(wrapper)

    expect(wrapper.get('[data-testid="agents-pane-error"]').text())
      .toBe('the session exited immediately; check that the agent CLI is installed and on PATH')
  })

  // The idle pane is a zero state, not a form: it explains itself and hands
  // the new-chat gesture to the same dialog the sidebar's + opens.
  it('offers the new-chat dialog from the idle pane', async () => {
    const { wrapper } = await mountAgentsMode('/workspaces/web-app')
    expect(wrapper.find('[data-testid="agents-pane-empty"]').exists()).toBe(true)
    expect(wrapper.findComponent(NewChatDialog).exists()).toBe(false)

    await wrapper.get('[data-testid="agents-new-session-open"]').trigger('click')

    expect(wrapper.findComponent(NewChatDialog).exists()).toBe(true)
  })

  // The + is a dialog opener, not a launch: nothing starts until the dialog
  // submits, and what it submits is what starts.
  it('starts the chat the dialog names, in the workspace it names', async () => {
    const client = fakeClient()
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    const { wrapper } = await mountAgentsMode('/workspaces/web-app')

    wrapper.findComponent(AgentsSidebar).vm.$emit('request-new-session')
    await flushPromises()
    expect(client.startSession).not.toHaveBeenCalled()
    expect(wrapper.findComponent(NewChatDialog).props('initialWorkspace')).toBe('web-app')

    wrapper.findComponent(NewChatDialog).vm.$emit('submit', { workspace: 'api', name: 'Ship it' })
    await flushPromises()

    expect(client.startSession).toHaveBeenCalledWith(expect.objectContaining({ workspace: 'api', name: 'Ship it' }))
    expect(wrapper.findComponent(NewChatDialog).exists()).toBe(false)
  })

  // The canvas is a sibling pane: opening and closing it must never re-key or
  // unmount the terminal host — the same element survives the round trip.
  it('keeps the terminal pane element across a canvas toggle', async () => {
    const { wrapper, router } = await mountWithOpenChat()
    const paneBefore = wrapper.find('[data-testid="agents-session-pane"]').element

    await setCanvas(router, '1')
    expect(router.currentRoute.value.query.canvas).toBe('1')
    expect(wrapper.find('[data-testid="agent-canvas-pane"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="agents-session-pane"]').element).toBe(paneBefore)

    await setCanvas(router, undefined)
    expect(router.currentRoute.value.query.canvas).toBeUndefined()
    expect(wrapper.find('[data-testid="agent-canvas-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="agents-session-pane"]').element).toBe(paneBefore)
  })

  // ?canvas names a view of the open chat, so with no chat open it renders
  // nothing — and closing the chat takes the query along.
  it('renders no canvas without an open chat, and drops ?canvas with the chat', async () => {
    const { wrapper: idle } = await mountAgentsMode('/workspaces/web-app?canvas=1')
    expect(idle.find('[data-testid="agent-canvas-pane"]').exists()).toBe(false)

    const { wrapper, router } = await mountWithOpenChat()
    await setCanvas(router, '1')
    expect(router.currentRoute.value.query.canvas).toBe('1')

    wrapper.findComponent(AgentsSidebar).vm.$emit('close-session', { ...chatRow })
    await flushPromises()

    expect(router.currentRoute.value.query.chat).toBeUndefined()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()
  })

  // open_canvas / close_canvas arrive as canvas:toggle. Only the open chat's
  // ask is honored — an agent must never drag the user away from another chat.
  it('opens and closes the pane on canvas:toggle for the open chat only', async () => {
    const { wrapper, router } = await mountWithOpenChat()

    wailsEvents.fire('canvas:toggle', { session: 99, name: '', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()

    wailsEvents.fire('canvas:toggle', { session: 7, name: 'plan', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBe('plan')
    expect(wrapper.find('[data-testid="agent-canvas-pane"]').exists()).toBe(true)

    wailsEvents.fire('canvas:toggle', { session: 7, name: '', open: false })
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()
    expect(wrapper.find('[data-testid="agent-canvas-pane"]').exists()).toBe(false)
  })

  // An unnamed chat is still a named chat — the default is applied at launch,
  // not left to the backend.
  it('defaults an unnamed chat to New Chat', async () => {
    const client = fakeClient()
    mocks.createAgentWorkspacesClient.mockReturnValue(client)
    const { wrapper } = await mountAgentsMode('/workspaces/web-app')

    await startChat(wrapper)

    expect(client.startSession).toHaveBeenCalledWith(expect.objectContaining({ name: 'New Chat' }))
  })
})
