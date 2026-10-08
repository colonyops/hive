import { flushPromises, mount, type DOMWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import TerminalMode from '../TerminalMode.vue'
import { useTerminalSessions } from '../../stores/useTerminalSessions'
import { resetSessionStatusesForTests, useSessionStatuses } from '../../composables/useSessionStatuses'
import { useTerminalShowWindows } from '../../stores/useTerminalShowWindows'
import { useTerminalStatusBar } from '../../stores/useTerminalStatusBar'
import { resetTerminalWindowListingsForTests } from '../../composables/useTerminalWindowListings'
import { useTerminalPinnedChats } from '../../stores/useTerminalPinnedChats'
import {
  resetAttachedTerminalWindowsForTests,
  useAttachedTerminalWindows,
} from '../../composables/useAttachedTerminalWindows'
import { useCommandPalette } from '../../composables/useCommands'
import {
  closeTerminalPane,
  closeTerminalWindow,
  focusTerminalFilter,
  newTerminalWindow,
  paneMayAutoFocus,
  selectTerminalWindow,
  stepTerminalWindow,
} from '../../lib/terminalTree'
import { createAppRouter } from '../../router'
import { tooltipFor } from '../../test-utils/tooltip'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  Scratch: vi.fn(),
  ListSessions: vi.fn(),
  On: vi.fn().mockReturnValue(() => {}),
  SessionStatuses: vi.fn(),
  SessionLaunchOptions: vi.fn(),
  SessionDetail: vi.fn(),
  SessionGitStatus: vi.fn(),
  SessionPullRequest: vi.fn(),
  OpenSessionInEditor: vi.fn(),
  RevealSession: vi.fn(),
  SessionRisk: vi.fn(),
  RenameSession: vi.fn(),
  DeleteSession: vi.fn(),
  RecycleSession: vi.fn(),
  PruneSessions: vi.fn(),
  TerminalActionViews: vi.fn(),
  InvokeTerminalAction: vi.fn(),
  RenderTerminalClipboardAction: vi.fn(),
  SetClipboardText: vi.fn(),
  getTerminalEndpoint: vi.fn(),
  createTerminalClient: vi.fn(),
  useTerminalWindows: vi.fn(),
  openBlank: vi.fn(),
  SetTerminalFontSize: vi.fn(),
  AppearanceSettings: vi.fn(),
  EditorSettings: vi.fn(),
  OpenURL: vi.fn(),
  AgentsAvailable: vi.fn(),
  allSessions: vi.fn(),
  resumeSession: vi.fn(),
}))

// The Code view reads the Agents area's own listing for its pinned chats, so the
// agents transport is mocked here too — a chat is not in hive's session list and
// cannot arrive through ListSessions.
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.AgentsAvailable,
  Endpoint: vi.fn().mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:2', wsURL: 'ws://127.0.0.1:2/s', token: 'a' }),
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()
  return {
    ...actual,
    createAgentWorkspacesClient: () => ({
      workspaces: vi.fn().mockResolvedValue({
        root: '',
        agents: [],
        editor: { command: '', title: '' },
        presets: [],
        rootProblem: '',
        workspaces: [],
      }),
      allSessions: mocks.allSessions,
      resumeSession: mocks.resumeSession,
    }),
  }
})

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice', () => ({
  Available: mocks.Available,
  Scratch: mocks.Scratch,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  EditorSettings: mocks.EditorSettings,
  SetEditor: vi.fn(),
  SetTerminalFontSize: mocks.SetTerminalFontSize,
  SetTerminalShowWindows: vi.fn(),
  SetTerminalShowStatusBar: vi.fn(),
  SetTerminalSessionAge: vi.fn(),
  SetTerminalPoolSize: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: vi.fn().mockResolvedValue(true),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  ListSessions: mocks.ListSessions,
  SessionStatuses: mocks.SessionStatuses,
  SessionLaunchOptions: mocks.SessionLaunchOptions,
  SessionDetail: mocks.SessionDetail,
  SessionGitStatus: mocks.SessionGitStatus,
  SessionPullRequest: mocks.SessionPullRequest,
  OpenSessionInEditor: mocks.OpenSessionInEditor,
  RevealSession: mocks.RevealSession,
  SessionRisk: mocks.SessionRisk,
  RenameSession: mocks.RenameSession,
  DeleteSession: mocks.DeleteSession,
  RecycleSession: mocks.RecycleSession,
  PruneSessions: mocks.PruneSessions,
  TerminalActionViews: mocks.TerminalActionViews,
  InvokeTerminalAction: mocks.InvokeTerminalAction,
  RenderTerminalClipboardAction: mocks.RenderTerminalClipboardAction,
}))
vi.mock('../../lib/terminalClient', () => ({
  getTerminalEndpoint: mocks.getTerminalEndpoint,
  createTerminalClient: mocks.createTerminalClient,
}))
vi.mock('../../composables/useTerminalWindows', () => ({
  useTerminalWindows: mocks.useTerminalWindows,
}))
vi.mock('../../composables/useNewSession', () => ({
  useNewSession: () => ({ openBlank: mocks.openBlank, prefetch: vi.fn() }),
}))
vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Clipboard: { SetText: mocks.SetClipboardText },
  Browser: { OpenURL: mocks.OpenURL },
}))

// The sidebar sweeps every session in one call, so a listing mock answers a map
// keyed by slug. A slug with no tmux session behind it is simply absent.
type FakeWindow = { windowId: string; name: string; active: boolean; width: number; height: number }
function fakeListWindows(bySlug: Record<string, FakeWindow[]> = {}) {
  return vi.fn(async (slugs: string[]) =>
    Object.fromEntries(slugs.filter((slug) => bySlug[slug]).map((slug) => [slug, bySlug[slug]])),
  )
}

/** The same window set for whichever sessions the sweep asks about. */
function fakeListWindowsEach(windows: FakeWindow[]) {
  return vi.fn(async (slugs: string[]) => Object.fromEntries(slugs.map((slug) => [slug, windows])))
}

type FakePane = { uid: number; paneId: string; term: object; scrolledUp: boolean }

function fakeSession() {
  return {
    tabs: ref([
      {
        uid: 1,
        windowId: '@1',
        name: 'agent',
        active: true,
        activePane: '%1',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [] as FakePane[],
      },
      {
        uid: 2,
        windowId: '@2',
        name: 'shell',
        active: false,
        activePane: '%2',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [] as FakePane[],
      },
    ]),
    activeWindowId: ref('@1'),
    status: ref<'connecting' | 'live' | 'ended'>('live'),
    painted: ref(true),
    endReason: ref<string | null>(null),
    error: ref<string | null>(null),
    actionError: ref<string | null>(null),
    sizeConstraint: ref<{ voted: { cols: number; rows: number }; granted: { cols: number; rows: number } } | null>(
      null,
    ),
    dismissSizeConstraint: vi.fn(),
    outputDropped: ref(false),
    dismissOutputDropped: vi.fn(),
    search: ref({ open: false, query: '', matches: 0, index: 0 }),
    openSearch: vi.fn(),
    closeSearch: vi.fn(),
    setSearchQuery: vi.fn(),
    findNext: vi.fn(),
    findPrevious: vi.fn(),
    start: vi.fn().mockResolvedValue(undefined),
    reconnect: vi.fn().mockResolvedValue(undefined),
    select: vi.fn().mockResolvedValue(undefined),
    newWindow: vi.fn().mockResolvedValue(undefined),
    newAgentWindow: vi.fn().mockResolvedValue(undefined),
    closeWindow: vi.fn().mockResolvedValue(undefined),
    rename: vi.fn().mockResolvedValue(undefined),
    moveWindow: vi.fn().mockResolvedValue(undefined),
    cell: ref(null),
    attachTab: vi.fn(),
    attachPane: vi.fn(),
    disposeTab: vi.fn(),
    selectPane: vi.fn().mockResolvedValue(undefined),
    splitPane: vi.fn().mockResolvedValue(undefined),
    closePane: vi.fn().mockResolvedValue(undefined),
    zoomPane: vi.fn().mockResolvedValue(undefined),
    focusPane: vi.fn().mockResolvedValue(undefined),
    resizePane: vi.fn().mockResolvedValue(undefined),
    focusActive: vi.fn(),
    scrollToBottom: vi.fn(),
    dispose: vi.fn(),
  }
}

// The pane chords act on the active window's active pane, which the plain
// fixture leaves without one.
function onePaneSession() {
  const session = fakeSession()
  session.tabs.value[0].panes = [{ uid: 11, paneId: '%1', term: {}, scrolledUp: false }]
  return session
}

// The scratch terminal's row is pinned above the repositories and is a session
// row like any other, so a test that means a *hive* session says so rather than
// taking whatever the tree lists first.
const SCRATCH_SLUG = 'Scratch'
function sessionRows(wrapper: { findAll: (s: string) => DOMWrapper<Element>[] }): DOMWrapper<Element>[] {
  return wrapper
    .findAll('[data-testid="terminal-session-row"]')
    .filter((row) => row.attributes('data-slug') !== SCRATCH_SLUG)
}

function openRowMenu(
  wrapper: { get: (s: string) => Pick<DOMWrapper<Element>, 'trigger'> },
  slug: string,
): Promise<void> {
  return wrapper
    .get(`[data-testid="terminal-session-row"][data-slug="${slug}"] [data-testid="terminal-session-menu-toggle"]`)
    .trigger('click')
}

// What is actually on screen: every pooled session's panes stay mounted, and
// only the visible session's active window is shown. Read off v-show's own
// inline style — isVisible() resolves through getComputedStyle, which happy-dom
// does not derive from it, so it answers true for a hidden pane.
function shownWindow(wrapper: { findAll: (s: string) => DOMWrapper<Element>[] }): string | undefined {
  return wrapper
    .findAll('[data-testid="terminal-pane"]')
    .find((pane) => (pane.element as HTMLElement).style.display !== 'none')
    ?.attributes('data-window-id')
}

// TerminalMode only renders under the terminal route in App.vue, so every
// mount starts there; tests deep-link by passing the session in the path.
// Transitions are stubbed because happy-dom never fires transitionend, which
// would leave leave-transitioned elements in the DOM past their v-if.
async function mountAt(path = '/terminal') {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()
  const wrapper = mount(TerminalMode, {
    global: { plugins: [router], stubs: { transition: true, 'transition-group': true } },
  })
  await flushPromises()
  return { wrapper, router }
}

async function mountAvailable(session = fakeSession()) {
  mocks.useTerminalWindows.mockReturnValue(session)
  const { wrapper, router } = await mountAt()
  return { wrapper, router, session }
}

function storedRestore() {
  return JSON.parse(localStorage.getItem('hive.terminal.restore') ?? 'null')
}

describe('TerminalMode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    resetSessionStatusesForTests()
    resetTerminalWindowListingsForTests()
    resetAttachedTerminalWindowsForTests()
    // The bar ships on; the pane's tests start with it off and turn it on by name.
    useTerminalStatusBar().setShowStatusBar(false)
    paneMayAutoFocus.value = true
    // The Agents area answers unavailable by default, which is what a build with
    // the experimental gate off looks like: no chats to pin, no Chats section.
    mocks.AgentsAvailable.mockResolvedValue({ available: false, reason: 'gated off' })
    mocks.allSessions.mockResolvedValue([])
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.Scratch.mockResolvedValue({ slug: 'Scratch', name: 'Terminals' })
    mocks.SessionLaunchOptions.mockResolvedValue({ agents: ['claude', 'codex'], defaultAgent: 'claude' })
    mocks.getTerminalEndpoint.mockResolvedValue({
      httpBaseURL: 'http://127.0.0.1:1',
      wsURL: 'ws://127.0.0.1:1/s',
      token: 't',
    })
    // Every close asks what the tab is running first; idle is the answer that
    // keeps the rest of these tests closing on the click.
    mocks.createTerminalClient.mockReturnValue({
      windowForeground: vi.fn().mockResolvedValue({ running: false, command: '' }),
    })
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'fix the parser', slug: 'hive-fix-parser', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'bump deps', slug: 'hive-bump-deps', repo: 'hay-kot/hive', state: 'active' },
    ])
    // Both fixtures run, which is what keeps their repo group expanded by
    // default — a group with nothing live in it opens only on a click.
    mocks.SessionStatuses.mockResolvedValue({
      items: [
        { sessionId: '1', running: true, windows: [] },
        { sessionId: '2', running: true, windows: [] },
      ],
      pollIntervalMs: 60_000,
    })
    mocks.SessionRisk.mockResolvedValue({ uncommittedChanges: false, unpushedCommits: false, recycleDeletes: false })
    mocks.TerminalActionViews.mockResolvedValue([])
    // The bar ships on, but the fixture keeps it off so tests that do not care
    // about it see a bare terminal; a test that wants it turns it on.
    mocks.AppearanceSettings.mockResolvedValue({
      theme: '',
      terminalFontSizePx: 13,
      terminalShowWindows: true,
      terminalShowStatusBar: false,
      terminalPoolSize: 3,
      terminalShowSessionAge: true,
      terminalSessionAgeThresholdDays: 5,
    })
    mocks.EditorSettings.mockResolvedValue({ command: 'zed', title: 'Zed', choices: [] })
    mocks.SessionGitStatus.mockResolvedValue({
      path: '/tmp/fix-parser',
      branch: 'feat/parser',
      dirty: false,
      unpushed: false,
      additions: 0,
      deletions: 0,
      host: 'github.com',
      owner: 'hay-kot',
      repo: 'hive',
      resolved: true,
      error: '',
    })
    mocks.SessionPullRequest.mockResolvedValue({ status: 'none' })
  })

  it('renders the unavailable panel with the reason instead of gating the mode', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'tmux is not installed.' })

    const { wrapper } = await mountAt()

    const panel = wrapper.find('[data-testid="terminal-unavailable"]')
    expect(panel.exists()).toBe(true)
    expect(wrapper.get('[data-testid="terminal-unavailable-reason"]').text()).toBe('tmux is not installed.')
    expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(false)
  })

  it('treats an endpoint failure as unavailable and can retry', async () => {
    mocks.getTerminalEndpoint.mockRejectedValueOnce(
      Object.assign(new Error('call failed'), {
        cause: { kind: 'unavailable', message: 'The local HTTP server is not running.' },
      }),
    )

    const { wrapper } = await mountAt()
    expect(wrapper.get('[data-testid="terminal-unavailable-reason"]').text()).toBe(
      'The local HTTP server is not running.',
    )

    mocks.useTerminalWindows.mockReturnValue(fakeSession())
    await wrapper.get('[data-testid="terminal-retry"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(true)
  })

  it('groups sessions by repo in the sidebar and attaches to the one picked', async () => {
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    const { wrapper, router } = await mountAt()

    expect(wrapper.find('[data-testid="terminal-no-session"]').exists()).toBe(true)
    const groups = wrapper.findAll('[data-testid="terminal-repo-group"]')
    expect(groups).toHaveLength(1)
    expect(groups[0].text()).toContain('hay-kot/hive')

    const rows = sessionRows(wrapper)
    expect(rows).toHaveLength(2)
    // Alphabetical within a group, like the TUI tree.
    expect(rows[0].text()).toContain('bump deps')

    await rows[1].trigger('click')
    await flushPromises()

    // The row navigates; the route is what attaches.
    expect(router.currentRoute.value.params.slug).toBe('hive-fix-parser')
    expect(mocks.useTerminalWindows).toHaveBeenCalledWith('hive-fix-parser', expect.anything())
    expect(session.start).toHaveBeenCalled()
    // The sidebar stays; the picked row is marked attached.
    expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="terminal-no-session"]').exists()).toBe(false)
    expect(rows[1].attributes('data-attached')).toBe('true')
    expect(wrapper.findAll('[data-testid="terminal-window-row"]').map((row) => row.text())).toEqual(['agent', 'shell'])
    expect(wrapper.findAll('[data-testid="terminal-pane"]')).toHaveLength(2)
  })

  it('marks sessions that have reached the configured age', async () => {
    const day = 24 * 60 * 60 * 1000
    mocks.ListSessions.mockResolvedValue([
      {
        id: '1',
        name: 'old session',
        slug: 'old-session',
        repo: 'hay-kot/hive',
        state: 'active',
        createdAt: new Date(Date.now() - 6 * day).toISOString(),
      },
      {
        id: '2',
        name: 'recent session',
        slug: 'recent-session',
        repo: 'hay-kot/hive',
        state: 'active',
        createdAt: new Date(Date.now() - 2 * day).toISOString(),
      },
    ])
    mocks.AppearanceSettings.mockResolvedValue({
      theme: '',
      terminalFontSizePx: 13,
      terminalShowWindows: true,
      terminalShowStatusBar: false,
      terminalPoolSize: 3,
      terminalShowSessionAge: true,
      terminalSessionAgeThresholdDays: 5,
    })
    mocks.useTerminalWindows.mockReturnValue(fakeSession())

    const { wrapper } = await mountAt()
    const old = wrapper.get('[data-testid="terminal-session-row"][data-slug="old-session"]')
    const recent = wrapper.get('[data-testid="terminal-session-row"][data-slug="recent-session"]')

    expect(old.get('[data-testid="terminal-session-age"]').text()).toBe('6d')
    expect(old.get('[data-testid="terminal-session-age"]').attributes('title')).toMatch(/^Started .*; 6 days old$/)
    expect(recent.find('[data-testid="terminal-session-age"]').exists()).toBe(false)
  })

  it('shows session liveness on sessions and agent activity on windows with icons', async () => {
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'live', slug: 'live', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'stopped', slug: 'stopped', repo: 'hay-kot/hive', state: 'active' },
    ])
    mocks.createTerminalClient.mockReturnValue({
      listWindows: fakeListWindows({
        live: [
          { windowId: '@1', name: 'working', active: true, width: 0, height: 0 },
          { windowId: '@2', name: 'approval', active: false, width: 0, height: 0 },
          { windowId: '@3', name: 'ready', active: false, width: 0, height: 0 },
          { windowId: '@4', name: 'unknown', active: false, width: 0, height: 0 },
        ],
      }),
    })
    const liveSession = fakeSession()
    liveSession.tabs.value = [
      {
        uid: 1,
        windowId: '@1',
        name: 'working',
        active: true,
        activePane: '%1',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
      {
        uid: 2,
        windowId: '@2',
        name: 'approval',
        active: false,
        activePane: '%2',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
      {
        uid: 3,
        windowId: '@3',
        name: 'ready',
        active: false,
        activePane: '%3',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
      {
        uid: 4,
        windowId: '@4',
        name: 'unknown',
        active: false,
        activePane: '%4',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
    ]
    mocks.useTerminalWindows.mockReturnValue(liveSession)
    mocks.SessionStatuses.mockResolvedValue({
      items: [
        {
          sessionId: '1',
          running: true,
          windows: [
            { windowId: '@1', status: 'active', tool: 'pi' },
            { windowId: '@2', status: 'approval', tool: 'claude' },
            { windowId: '@3', status: 'ready', tool: 'codex' },
            { windowId: '@4', status: 'missing', tool: '' },
          ],
        },
        { sessionId: '2', running: false, windows: [] },
      ],
      pollIntervalMs: 60_000,
    })

    const { wrapper } = await mountAt()
    // Only a running session carries a glyph; an idle one greys its name instead.
    const liveness = wrapper.findAll('[data-testid="terminal-session-liveness"]')
    expect(liveness).toHaveLength(1)
    expect(liveness[0].text()).toBe('Terminal running')
    expect(liveness[0].get('span[aria-hidden="true"]').classes()).toEqual(
      expect.arrayContaining(['size-2.5', 'rounded-full', 'bg-current']),
    )
    const idleRow = wrapper.get('[data-testid="terminal-session-row"][data-slug="stopped"]')
    expect(idleRow.find('[data-testid="terminal-session-liveness"]').exists()).toBe(false)
    expect(idleRow.get('span').classes()).toContain('text-text-3')
    const liveRow = wrapper.get('[data-testid="terminal-session-row"][data-slug="live"]')
    expect(liveRow.get('span').classes()).not.toContain('text-text-3')
    expect(liveRow.get('[data-testid="terminal-session-liveness"]').element.parentElement).toBe(
      liveRow.get('[data-testid="terminal-session-menu-toggle"]').element.parentElement,
    )

    const activity = wrapper.findAll('[data-testid="terminal-window-status"]')
    expect(activity).toHaveLength(4)
    expect(activity.every((indicator) => indicator.find('svg').exists())).toBe(true)
    expect(activity.every((indicator) => indicator.classes().includes('window-status'))).toBe(true)
    expect(wrapper.get('[data-testid="terminal-window-status"][data-status="active"]').attributes('title')).toBe(
      'pi is working',
    )
    expect(wrapper.get('[data-testid="terminal-window-status"][data-status="active"] svg').classes()).toContain(
      'animate-spin',
    )
    expect(wrapper.get('[data-testid="terminal-window-status"][data-status="approval"]').attributes('title')).toBe(
      'claude needs approval',
    )
    expect(wrapper.text()).not.toContain('[●]')

    await wrapper.get('[data-testid="terminal-session-row"][data-slug="live"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('[data-testid="terminal-window-row"]')).toHaveLength(4)
    expect(wrapper.findAll('[data-testid="terminal-window-status"]')).toHaveLength(4)
  })

  it('opens the new-session dialog from the sidebar header', async () => {
    const { wrapper } = await mountAvailable()
    await wrapper.get('[data-testid="terminal-new-session"]').trigger('click')
    expect(mocks.openBlank).toHaveBeenCalledOnce()
    // Nothing is attached yet, so there is no repository to inherit.
    expect(mocks.openBlank).toHaveBeenCalledWith('')
  })

  it('starts a new session in the attached session’s repository', async () => {
    const { wrapper } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="terminal-new-session"]').trigger('click')
    expect(mocks.openBlank).toHaveBeenCalledWith('hay-kot/hive')
  })

  it('starts a new session in the repository whose header was clicked', async () => {
    const { wrapper } = await mountAvailable()
    await wrapper.get('[data-testid="terminal-repo-new-session"]').trigger('click')
    expect(mocks.openBlank).toHaveBeenCalledWith('hay-kot/hive')
  })

  // The group stays collapsed-or-not: the button is inside the header, and a
  // click that reached the header would toggle the whole repository shut.
  it('does not toggle the group when its add button is clicked', async () => {
    const { wrapper } = await mountAvailable()
    await wrapper.get('[data-testid="terminal-repo-new-session"]').trigger('click')
    expect(sessionRows(wrapper)).toHaveLength(2)
  })

  it('collapses a repo group without losing the attached session', async () => {
    const { wrapper } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="terminal-repo-group"]').trigger('click')
    expect(sessionRows(wrapper)).toHaveLength(0)
    expect(wrapper.findAll('[data-testid="terminal-pane"]')).toHaveLength(2)

    await wrapper.get('[data-testid="terminal-repo-group"]').trigger('click')
    expect(sessionRows(wrapper)).toHaveLength(2)
  })

  // A machine's worth of repos would otherwise open at once and bury the one
  // being worked in, so an untouched group takes its state from its sessions.
  it('opens an untouched repo group only when something in it is running', async () => {
    mocks.SessionStatuses.mockResolvedValue({
      items: [
        { sessionId: '1', running: false, windows: [] },
        { sessionId: '2', running: false, windows: [] },
      ],
      pollIntervalMs: 60_000,
    })

    const { wrapper } = await mountAt()

    expect(wrapper.get('[data-testid="terminal-repo-group"]').attributes('aria-expanded')).toBe('false')
    expect(sessionRows(wrapper)).toHaveLength(0)
  })

  // The tree renders from cache a poll before the first status does, so a repo
  // that waited on one would collapse under the session already on screen.
  it('opens the attached repo group before any status has arrived', async () => {
    mocks.SessionStatuses.mockResolvedValue({ items: [], pollIntervalMs: 60_000 })
    mocks.useTerminalWindows.mockReturnValue(fakeSession())

    const { wrapper } = await mountAt('/terminal/hive-fix-parser')

    expect(wrapper.get('[data-testid="terminal-repo-group"]').attributes('aria-expanded')).toBe('true')
  })

  // Once toggled, the choice is the user's and outlives what the sessions do.
  it('keeps a collapsed group collapsed while its sessions run', async () => {
    const { wrapper } = await mountAt()
    await wrapper.get('[data-testid="terminal-repo-group"]').trigger('click')

    const { wrapper: reopened } = await mountAt()

    expect(reopened.get('[data-testid="terminal-repo-group"]').attributes('aria-expanded')).toBe('false')
  })

  // The markers are two elements that move, so what a test can pin is which row
  // each one is bound to — the travel itself is a layout the DOM here has none
  // of, and is verified by eye like the rest of the tree's motion.
  it('binds a selection rail to the attached session and to its active window', async () => {
    const { wrapper } = await mountAvailable()

    expect(wrapper.get('[data-testid="terminal-session-rail"]').attributes('data-shown')).toBe('false')
    expect(wrapper.get('[data-testid="terminal-window-rail"]').attributes('data-shown')).toBe('false')

    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="terminal-session-rail"]').attributes('data-shown')).toBe('true')
    expect(wrapper.get('[data-testid="terminal-window-rail"]').attributes('data-shown')).toBe('true')
  })

  // A rail whose row left the tree fades where it stands; resetting it would
  // make the next selection travel from the top of the list.
  it('hides a rail without moving it when its row goes away', async () => {
    const { wrapper } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    const placed = wrapper.get('[data-testid="terminal-session-rail"]').attributes('style')
    await wrapper.get('[data-testid="terminal-repo-group"]').trigger('click')
    await flushPromises()

    const rail = wrapper.get('[data-testid="terminal-session-rail"]')
    expect(rail.attributes('data-shown')).toBe('false')
    expect(rail.attributes('style')).toBe(placed)
  })

  it('nests the attached session’s windows in the tree and selects from them', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    const windows = wrapper.findAll('[data-testid="terminal-window-row"]')
    expect(windows).toHaveLength(2)
    expect(windows[0].attributes('data-active')).toBe('true')

    await windows[1].trigger('click')
    expect(session.select).toHaveBeenCalledWith('@2')
  })

  // The keymap dispatches ⌘1…⌘9 against the strip's positions; a session with
  // fewer windows than the digit ignores the chord rather than clamping.
  it('jumps to a window by position, and past the end does nothing', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    selectTerminalWindow(2)
    await flushPromises()
    expect(session.select).toHaveBeenCalledWith('@2')
    expect(paneMayAutoFocus.value).toBe(true)

    session.select.mockClear()
    selectTerminalWindow(3)
    await flushPromises()
    expect(session.select).not.toHaveBeenCalled()
  })

  // Wrapping is the difference from the numbered jumps: a session's windows are
  // a ring, so next from the last lands on the first rather than doing nothing.
  it('walks the attached session’s windows and wraps at either end', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    stepTerminalWindow(1)
    await flushPromises()
    expect(session.select).toHaveBeenCalledWith('@2')
    expect(paneMayAutoFocus.value).toBe(true)

    session.select.mockClear()
    session.activeWindowId.value = '@2'
    stepTerminalWindow(1)
    await flushPromises()
    expect(session.select).toHaveBeenCalledWith('@1')

    session.select.mockClear()
    stepTerminalWindow(-1)
    await flushPromises()
    expect(session.select).toHaveBeenCalledWith('@1')
  })

  it('opens and closes windows on the attached session from the keymap', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    newTerminalWindow()
    await flushPromises()
    // Through the attached session's own client, which is what makes the new
    // window the active one.
    expect(session.newWindow).toHaveBeenCalled()
    expect(paneMayAutoFocus.value).toBe(true)

    closeTerminalWindow()
    await flushPromises()
    expect(session.closeWindow).toHaveBeenCalledWith('@1')
  })

  // Nothing is attached before a row is picked, so the chords have no window to
  // name and must not reach for one.
  it('ignores the window chords while no session is attached', async () => {
    const { session } = await mountAvailable()

    newTerminalWindow()
    closeTerminalWindow()
    stepTerminalWindow(1)
    await flushPromises()

    expect(session.newWindow).not.toHaveBeenCalled()
    expect(session.closeWindow).not.toHaveBeenCalled()
    expect(session.select).not.toHaveBeenCalled()
  })

  it('keeps the outgoing attach warm and snaps back to it without re-attaching', async () => {
    const first = fakeSession()
    const second = fakeSession()
    second.tabs.value = [
      {
        uid: 9,
        windowId: '@9',
        name: 'other',
        active: true,
        activePane: '%9',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
    ]
    second.activeWindowId.value = '@9'
    mocks.useTerminalWindows.mockReturnValueOnce(first).mockReturnValueOnce(second)
    const { wrapper } = await mountAt()
    const rows = sessionRows(wrapper)
    await rows[0].trigger('click')
    await flushPromises()

    // Re-clicking the attached row must not re-attach.
    await wrapper.find('[data-testid="terminal-session-row"][data-attached="true"]').trigger('click')
    await flushPromises()
    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(1)

    await rows[1].trigger('click')
    await flushPromises()
    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(2)
    // The outgoing attach stays in the pool: nothing disposed, its panes still
    // mounted (hidden) beside the incoming session's, its subtree still live.
    expect(first.dispose).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-testid="terminal-pane"]')).toHaveLength(3)
    expect(wrapper.findAll('[data-testid="terminal-window-row"]')).toHaveLength(3)
    expect(shownWindow(wrapper)).toBe('@9')

    // Snapping back is a reveal and a refocus, not a re-attach.
    await rows[0].trigger('click')
    await flushPromises()
    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(2)
    expect(first.focusActive).toHaveBeenCalled()
    expect(shownWindow(wrapper)).toBe('@1')
  })

  it('keeps the pool through a trip to the hub, so re-entry re-attaches nothing', async () => {
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    const { wrapper, router } = await mountAt()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()
    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(1)

    // The mode is hidden, never unmounted (App.vue), so leaving it is the
    // `active` prop going false alongside the route change.
    await router.push('/feed')
    await wrapper.setProps({ active: false })
    await flushPromises()
    expect(session.dispose).not.toHaveBeenCalled()

    await router.push('/terminal/hive-bump-deps')
    await wrapper.setProps({ active: true })
    await flushPromises()

    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(1)
    expect(session.start).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('[data-testid="terminal-pane"]')).toHaveLength(2)
  })

  it('holds the outgoing session on screen until the incoming one paints', async () => {
    const first = fakeSession()
    const second = fakeSession()
    second.tabs.value = []
    second.status.value = 'connecting'
    second.painted.value = false
    mocks.useTerminalWindows.mockReturnValueOnce(first).mockReturnValueOnce(second)
    const { wrapper } = await mountAt()
    const rows = sessionRows(wrapper)
    await rows[0].trigger('click')
    await flushPromises()

    await rows[1].trigger('click')
    await flushPromises()

    // The sidebar reflects the selection at once; the pane does not blank.
    expect(wrapper.find('[data-testid="terminal-session-row"][data-attached="true"]').attributes('data-slug')).toBe(
      'hive-fix-parser',
    )
    expect(shownWindow(wrapper)).toBe('@1')

    // First paint is the swap signal.
    second.tabs.value = [
      {
        uid: 9,
        windowId: '@9',
        name: 'other',
        active: true,
        activePane: '%9',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
    ]
    second.activeWindowId.value = '@9'
    second.status.value = 'live'
    second.painted.value = true
    await flushPromises()
    expect(shownWindow(wrapper)).toBe('@9')
    expect(second.focusActive).toHaveBeenCalled()
  })

  it('gives up the hold once the cap fires, so a slow attach is not a dead click', async () => {
    vi.useFakeTimers()
    try {
      const first = fakeSession()
      const second = fakeSession()
      second.tabs.value = []
      second.status.value = 'connecting'
      second.painted.value = false
      mocks.useTerminalWindows.mockReturnValueOnce(first).mockReturnValueOnce(second)
      const { wrapper } = await mountAt()
      const rows = sessionRows(wrapper)
      await rows[0].trigger('click')
      await flushPromises()

      await rows[1].trigger('click')
      await flushPromises()
      expect(shownWindow(wrapper)).toBe('@1')

      await vi.advanceTimersByTimeAsync(300)
      expect(shownWindow(wrapper)).toBeUndefined()
      expect(wrapper.text()).toContain('Attaching…')
    } finally {
      vi.useRealTimers()
    }
  })

  it('detaches the least recently used session past the pool limit', async () => {
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'aaa', slug: 'hive-aaa', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'bbb', slug: 'hive-bbb', repo: 'hay-kot/hive', state: 'active' },
      { id: '3', name: 'ccc', slug: 'hive-ccc', repo: 'hay-kot/hive', state: 'active' },
      { id: '4', name: 'ddd', slug: 'hive-ddd', repo: 'hay-kot/hive', state: 'active' },
    ])
    const sessions = [fakeSession(), fakeSession(), fakeSession(), fakeSession()]
    for (const session of sessions) mocks.useTerminalWindows.mockReturnValueOnce(session)
    const { wrapper } = await mountAt()

    for (const row of sessionRows(wrapper)) {
      await row.trigger('click')
      await flushPromises()
    }

    expect(sessions[0].dispose).toHaveBeenCalledTimes(1)
    expect(sessions[1].dispose).not.toHaveBeenCalled()
    expect(sessions[2].dispose).not.toHaveBeenCalled()
    expect(sessions[3].dispose).not.toHaveBeenCalled()
  })

  it('does not bring back a window closed while attached once the session is evicted', async () => {
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'aaa', slug: 'hive-aaa', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'bbb', slug: 'hive-bbb', repo: 'hay-kot/hive', state: 'active' },
      { id: '3', name: 'ccc', slug: 'hive-ccc', repo: 'hay-kot/hive', state: 'active' },
      { id: '4', name: 'ddd', slug: 'hive-ddd', repo: 'hay-kot/hive', state: 'active' },
    ])
    const tmux: Record<string, FakeWindow[]> = {
      'hive-aaa': [
        { windowId: '@1', name: 'agent', active: true, width: 0, height: 0 },
        { windowId: '@2', name: 'shell', active: false, width: 0, height: 0 },
      ],
    }
    mocks.createTerminalClient.mockReturnValue({
      listWindows: vi.fn(async (slugs: string[]) =>
        Object.fromEntries(slugs.filter((slug) => tmux[slug]).map((slug) => [slug, tmux[slug]])),
      ),
    })
    const sessions = [fakeSession(), fakeSession(), fakeSession(), fakeSession()]
    for (const session of sessions) mocks.useTerminalWindows.mockReturnValueOnce(session)
    const { wrapper } = await mountAt()
    const rows = sessionRows(wrapper)

    await rows[0].trigger('click')
    await flushPromises()
    sessions[0].tabs.value = sessions[0].tabs.value.filter((tab) => tab.windowId !== '@2')
    tmux['hive-aaa'] = tmux['hive-aaa'].filter((win) => win.windowId !== '@2')
    await flushPromises()

    for (const row of rows.slice(1)) {
      await row.trigger('click')
      await flushPromises()
    }
    expect(sessions[0].dispose).toHaveBeenCalledTimes(1)

    const listed = wrapper
      .findAll('[data-testid="terminal-listed-window-row"]')
      .filter((row) => row.attributes('data-tree-key')?.startsWith('w:1:'))
    expect(listed.map((row) => row.text())).toEqual(['agent'])
  })

  it('refocuses the terminal when the attached row is reselected', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    await wrapper.find('[data-testid="terminal-session-row"][data-attached="true"]').trigger('click')
    await flushPromises()

    expect(session.focusActive).toHaveBeenCalled()
    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(1)
  })

  it('offers a way back to the live tail while the viewport is scrolled up', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="terminal-scroll-to-bottom"]').exists()).toBe(false)

    const first = session.tabs.value[0] as { panes: unknown[] }
    first.panes = [{ uid: 11, paneId: '%1', term: {}, scrolledUp: true }]
    await flushPromises()
    await wrapper.get('[data-testid="terminal-scroll-to-bottom"]').trigger('click')
    expect(session.scrollToBottom).toHaveBeenCalled()

    // Only the active window's viewport matters — and the ended overlay
    // replaces the pane, so the pill goes with it.
    session.activeWindowId.value = '@2'
    await flushPromises()
    expect(wrapper.find('[data-testid="terminal-scroll-to-bottom"]').exists()).toBe(false)

    session.activeWindowId.value = '@1'
    session.status.value = 'ended'
    await flushPromises()
    expect(wrapper.find('[data-testid="terminal-scroll-to-bottom"]').exists()).toBe(false)
  })

  it('lists windows for unattached sessions out of the box', async () => {
    const listWindows = fakeListWindows({
      'hive-bump-deps': [
        { windowId: '@7', name: 'agent', active: true, width: 0, height: 0 },
        { windowId: '@8', name: 'shell', active: false, width: 0, height: 0 },
      ],
    })
    mocks.createTerminalClient.mockReturnValue({ listWindows })
    const session = fakeSession()
    session.tabs.value = [
      {
        uid: 7,
        windowId: '@7',
        name: 'agent',
        active: true,
        activePane: '%7',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
      {
        uid: 8,
        windowId: '@8',
        name: 'shell',
        active: false,
        activePane: '%8',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
    ]
    session.activeWindowId.value = '@7'
    const { wrapper, router } = await mountAvailable(session)

    // Settings ▸ Terminal ships the listing on, so the tree fills
    // in without touching anything — and one call carries the whole sidebar,
    // the pinned scratch terminal included.
    expect(listWindows).toHaveBeenCalledWith(['Scratch', 'hive-fix-parser', 'hive-bump-deps'])
    const rows = wrapper.findAll('[data-testid="terminal-listed-window-row"]')
    expect(rows.map((row) => row.text())).toEqual(['agent', 'shell'])

    // A listed window is a deep link: attach the session with that window
    // pinned, exactly like entering /terminal/:slug?window=….
    await rows[1].trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
    expect(session.select).toHaveBeenCalledWith('@8')

    // Attached now, so its rows are the live tab set, not the listing.
    expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(0)
    expect(wrapper.findAll('[data-testid="terminal-window-row"]')).toHaveLength(2)
  })

  it('empties the tree of listed windows when the setting is turned off', async () => {
    const listWindows = fakeListWindowsEach([{ windowId: '@7', name: 'agent', active: true, width: 0, height: 0 }])
    mocks.createTerminalClient.mockReturnValue({ listWindows })
    const { wrapper } = await mountAvailable()
    // One per session and one for the scratch terminal: the sweep answers for
    // every slug it was asked about.
    expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(3)

    useTerminalShowWindows().setShowWindows(false)
    await flushPromises()
    // All but the pinned section's, which is exempt: its tabs are the section.
    expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="terminal-listed-window-row"]').attributes('data-tree-key')).toBe('w:Scratch:@7')
  })

  it('re-enters from the caches and resumes without waiting on the probe', async () => {
    const listWindows = fakeListWindowsEach([{ windowId: '@7', name: 'agent', active: true, width: 0, height: 0 }])
    mocks.createTerminalClient.mockReturnValue({ listWindows })
    const { wrapper } = await mountAvailable()
    expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(3)
    wrapper.unmount()

    // Neither the probe nor the listing resolves this time: everything the
    // second entry shows has to come from the caches, revalidation pending.
    mocks.Available.mockReturnValue(new Promise(() => {}))
    mocks.ListSessions.mockReturnValue(new Promise(() => {}))
    localStorage.setItem('hive.terminal.restore', JSON.stringify({ slug: 'hive-bump-deps', window: '' }))
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    const { wrapper: second, router } = await mountAt()

    expect(second.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(true)
    expect(sessionRows(second)).toHaveLength(2)
    // The refresh control is the staleness indicator while revalidation runs.
    expect(second.get('[data-testid="terminal-sessions-refresh"]').find('.animate-spin').exists()).toBe(true)
    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
    expect(mocks.useTerminalWindows).toHaveBeenCalledWith('hive-bump-deps', expect.anything())
    second.unmount()
  })

  it('stands in the cached listing for the subtree while attaching', async () => {
    const listWindows = fakeListWindowsEach([
      { windowId: '@7', name: 'agent', active: true, width: 0, height: 0 },
      { windowId: '@8', name: 'shell', active: false, width: 0, height: 0 },
    ])
    mocks.createTerminalClient.mockReturnValue({ listWindows })
    const session = fakeSession()
    session.tabs.value = []
    session.status.value = 'connecting'
    const { wrapper } = await mountAvailable(session)

    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    // The attached session's subtree stands in its cached rows until the live
    // ones arrive, so selecting a session does not empty the tree first.
    expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(6)

    // The live tab set replaces the stand-ins in place.
    session.tabs.value = [
      {
        uid: 7,
        windowId: '@7',
        name: 'agent',
        active: true,
        activePane: '%7',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
      {
        uid: 8,
        windowId: '@8',
        name: 'shell',
        active: false,
        activePane: '%8',
        width: 213,
        height: 55,
        zoomed: false,
        layout: null,
        panes: [],
      },
    ]
    session.status.value = 'live'
    await flushPromises()
    expect(wrapper.findAll('[data-testid="terminal-window-row"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(4)
  })

  it('says when there are no sessions to attach to', async () => {
    mocks.ListSessions.mockResolvedValue([])
    const { wrapper } = await mountAt()

    expect(wrapper.find('[data-testid="terminal-sessions-empty"]').exists()).toBe(true)
  })

  it('resumes the last attached session and window when entering bare', async () => {
    localStorage.setItem('hive.terminal.restore', JSON.stringify({ slug: 'hive-bump-deps', window: '@2' }))
    const { wrapper, router, session } = await mountAvailable()
    await flushPromises()

    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
    expect(mocks.useTerminalWindows).toHaveBeenCalledWith('hive-bump-deps', expect.anything())
    expect(session.select).toHaveBeenCalledWith('@2')
    expect(wrapper.find('[data-testid="terminal-no-session"]').exists()).toBe(false)
  })

  it('forgets a remembered session that no longer exists and offers the picker', async () => {
    localStorage.setItem('hive.terminal.restore', JSON.stringify({ slug: 'hive-long-gone', window: '@1' }))
    const { wrapper, router } = await mountAt()

    expect(mocks.useTerminalWindows).not.toHaveBeenCalled()
    expect(router.currentRoute.value.params.slug ?? '').toBe('')
    expect(wrapper.find('[data-testid="terminal-no-session"]').exists()).toBe(true)
    expect(storedRestore()).toEqual({ slug: '', window: '' })
  })

  it('attaches straight from a session deep link', async () => {
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    await mountAt('/terminal/hive-fix-parser')

    expect(mocks.useTerminalWindows).toHaveBeenCalledWith('hive-fix-parser', expect.anything())
    expect(session.start).toHaveBeenCalled()
  })

  it('falls back to tmux’s active window when the deep-linked one is gone', async () => {
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    await mountAt('/terminal/hive-fix-parser?window=@9')

    expect(session.select).not.toHaveBeenCalled()
  })

  // Finding: the App-level palette's window rows push the same slug with a
  // new ?window (useAppPaletteRows), which moves neither of the attach
  // watcher's sources nor the mirror's — so without its own watcher on
  // routeWindow, this push did nothing.
  it('selects the window a same-slug ?window push names on the pooled client', async () => {
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    const { router } = await mountAt('/terminal/hive-fix-parser')
    await flushPromises()
    session.select.mockClear()

    await router.push('/terminal/hive-fix-parser?window=@2')
    await flushPromises()

    expect(session.select).toHaveBeenCalledWith('@2')
  })

  it('mirrors the active window into the URL and the resume snapshot', async () => {
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    const { router } = await mountAt('/terminal/hive-fix-parser')
    await flushPromises()

    expect(router.currentRoute.value.query.window).toBe('@1')

    session.activeWindowId.value = '@2'
    await flushPromises()

    expect(router.currentRoute.value.query.window).toBe('@2')
    expect(storedRestore()).toEqual({ slug: 'hive-fix-parser', window: '@2' })
  })

  it('offers reconnect when the stream drops, and again when tmux exits', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="terminal-session-ended"]').exists()).toBe(false)

    session.status.value = 'ended'
    session.endReason.value = 'disconnected'
    session.error.value = 'The terminal stream disconnected.'
    await flushPromises()

    expect(wrapper.get('[data-testid="terminal-session-ended"]').text()).toContain('Terminal stream lost')
    expect(wrapper.get('[data-testid="terminal-session-ended-reason"]').text()).toBe(
      'The terminal stream disconnected.',
    )

    session.endReason.value = 'exited'
    session.error.value = 'overflow'
    await flushPromises()
    expect(wrapper.get('[data-testid="terminal-session-ended"]').text()).toContain('Session ended')

    await wrapper.get('[data-testid="terminal-reconnect"]').trigger('click')
    expect(session.reconnect).toHaveBeenCalled()
  })

  // The grid not matching the pane is tmux's rule, not a fault, so it is named
  // in place rather than reported as an error.
  it('names the client that is deciding the size, and can be dismissed', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="terminal-size-constraint"]').exists()).toBe(false)

    session.sizeConstraint.value = { voted: { cols: 213, rows: 55 }, granted: { cols: 80, rows: 24 } }
    await flushPromises()

    const hint = wrapper.get('[data-testid="terminal-size-constraint"]').text()
    expect(hint).toContain('80×24')
    expect(hint).toContain('213×55')
    expect(hint).toContain('another attached')

    await wrapper.get('[data-testid="terminal-size-constraint-dismiss"]').trigger('click')
    expect(session.dismissSizeConstraint).toHaveBeenCalled()
  })

  // The panes survive an overflow now, which is exactly why the gap has to be
  // said out loud: without it a resync looks like nothing happened.
  it('says so when output was dropped and the panes were repainted', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="terminal-output-dropped"]').exists()).toBe(false)

    session.outputDropped.value = true
    await flushPromises()

    const notice = wrapper.get('[data-testid="terminal-output-dropped"]').text()
    expect(notice).toContain('dropped')
    expect(notice).toContain('repainted from tmux')

    await wrapper.get('[data-testid="terminal-output-dropped-dismiss"]').trigger('click')
    expect(session.dismissOutputDropped).toHaveBeenCalled()
  })

  it('re-attaches from the sidebar row after the session ended', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    session.status.value = 'ended'
    await flushPromises()

    await wrapper.find('[data-testid="terminal-session-row"][data-attached="true"]').trigger('click')
    await flushPromises()
    expect(session.dispose).toHaveBeenCalledTimes(1)
    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(2)
  })

  it('drives the window controls from the tree and hands panes to the composable', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    expect(session.attachTab).toHaveBeenCalledTimes(2)
    expect(session.attachTab.mock.calls[0][0]).toBe('@1')

    // The attached session's row adds a window through its own client, which is
    // what makes the new one active.
    await wrapper
      .get('[data-testid="terminal-session-row"][data-attached="true"] [data-testid="terminal-new-window"]')
      .trigger('click')
    await wrapper.get('[data-testid="new-window-terminal"]').trigger('click')
    expect(session.newWindow).toHaveBeenCalled()

    await wrapper.findAll('[data-testid="terminal-close-window"]')[1].trigger('click')
    expect(session.closeWindow).toHaveBeenCalledWith('@2')

    const rows = wrapper.findAll('[data-testid="terminal-window-row"]')
    await rows[1].trigger('dblclick')
    const input = wrapper.get('[data-testid="terminal-rename-input"]')
    await input.setValue('build')
    await input.trigger('keydown.enter')
    expect(session.rename).toHaveBeenCalledWith('@2', 'build')
  })

  // Closing a tab kills its tmux window and everything in it, so a tab running
  // something is confirmed first — and the dialog names the process, because
  // "are you sure" is not a question the user can answer without it.
  it('confirms before closing a tab that is running something', async () => {
    const windowForeground = vi.fn().mockResolvedValue({ running: true, command: 'claude' })
    mocks.createTerminalClient.mockReturnValue({ windowForeground })
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    await wrapper.findAll('[data-testid="terminal-close-window"]')[0].trigger('click')
    await flushPromises()

    // Read at the moment of the close, for the window the close would kill.
    expect(windowForeground).toHaveBeenCalledWith('hive-bump-deps', '@1')
    expect(session.closeWindow).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="session-confirmation"]')?.textContent).toContain(
      'claude is still running in agent',
    )

    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-confirm"]')?.click()
    await flushPromises()
    expect(session.closeWindow).toHaveBeenCalledWith('@1')
  })

  it('leaves the tab and its process alone when the confirmation is cancelled', async () => {
    mocks.createTerminalClient.mockReturnValue({
      windowForeground: vi.fn().mockResolvedValue({ running: true, command: 'claude' }),
    })
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    await wrapper.findAll('[data-testid="terminal-close-window"]')[0].trigger('click')
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-cancel"]')?.click()
    await flushPromises()

    expect(session.closeWindow).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="session-confirmation"]')).toBeNull()
  })

  // The chord and the button are the same close, so they take the same guard —
  // the chord acts on the active window, which is the one it names.
  it('confirms before the close chord kills a running tab', async () => {
    const windowForeground = vi.fn().mockResolvedValue({ running: true, command: 'nvim' })
    mocks.createTerminalClient.mockReturnValue({ windowForeground })
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    closeTerminalWindow()
    await flushPromises()

    expect(windowForeground).toHaveBeenCalledWith('hive-bump-deps', '@1')
    expect(session.closeWindow).not.toHaveBeenCalled()
    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-confirm"]')?.click()
    await flushPromises()
    expect(session.closeWindow).toHaveBeenCalledWith('@1')
  })

  // A check that failed is not evidence the tab is idle, and killing a process
  // to find out is what the confirmation exists to prevent.
  it('confirms when what the tab is running could not be read', async () => {
    mocks.createTerminalClient.mockReturnValue({
      windowForeground: vi.fn().mockRejectedValue(new Error('tmux went away')),
    })
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    await wrapper.findAll('[data-testid="terminal-close-window"]')[0].trigger('click')
    await flushPromises()

    expect(session.closeWindow).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="session-confirmation"]')?.textContent).toContain(
      'Something is still running in agent',
    )
    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-cancel"]')?.click()
  })

  // Pane close shares the window-close safety policy and checks the target at
  // action time.
  it('confirms before the close-pane chord kills a running pane', async () => {
    const paneForeground = vi.fn().mockResolvedValue({ running: true, command: 'claude' })
    mocks.createTerminalClient.mockReturnValue({ paneForeground })
    const { wrapper, session } = await mountAvailable(onePaneSession())
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    closeTerminalPane()
    await flushPromises()

    expect(paneForeground).toHaveBeenCalledWith('hive-bump-deps', '%1')
    expect(session.closePane).not.toHaveBeenCalled()
    const dialog = document.querySelector('[data-testid="session-confirmation"]')?.textContent
    expect(dialog).toContain('Close this pane?')
    expect(dialog).toContain('claude is still running in agent')

    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-confirm"]')?.click()
    await flushPromises()
    expect(session.closePane).toHaveBeenCalledWith('%1')
  })

  it('leaves the pane and its process alone when the confirmation is cancelled', async () => {
    mocks.createTerminalClient.mockReturnValue({
      paneForeground: vi.fn().mockResolvedValue({ running: true, command: 'claude' }),
    })
    const { wrapper, session } = await mountAvailable(onePaneSession())
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    closeTerminalPane()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-cancel"]')?.click()
    await flushPromises()

    expect(session.closePane).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="session-confirmation"]')).toBeNull()
  })

  it('confirms when what the pane is running could not be read', async () => {
    mocks.createTerminalClient.mockReturnValue({
      paneForeground: vi.fn().mockRejectedValue(new Error('tmux went away')),
    })
    const { wrapper, session } = await mountAvailable(onePaneSession())
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    closeTerminalPane()
    await flushPromises()

    expect(session.closePane).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="session-confirmation"]')?.textContent).toContain(
      'Something is still running in agent',
    )
    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-cancel"]')?.click()
  })

  it('closes an idle pane on the chord without asking', async () => {
    const paneForeground = vi.fn().mockResolvedValue({ running: false, command: '' })
    mocks.createTerminalClient.mockReturnValue({ paneForeground })
    const { wrapper, session } = await mountAvailable(onePaneSession())
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    closeTerminalPane()
    await flushPromises()

    expect(paneForeground).toHaveBeenCalledWith('hive-bump-deps', '%1')
    expect(session.closePane).toHaveBeenCalledWith('%1')
    expect(document.querySelector('[data-testid="session-confirmation"]')).toBeNull()
  })

  // Adding a window is a property of the session, not of what is on screen, so
  // an unattached row offers it too — through the slug-keyed call, since there
  // is no control client to route it through yet.
  // The last window closing kills the tmux session, which empties the live tab
  // set that was standing in front of the listing. Nothing the listing sweep
  // watches moves on a death, so without a trigger of its own the subtree falls
  // back to a cached listing of windows tmux no longer holds.
  it('re-sweeps a subtree when its session ends, so dead windows leave the tree', async () => {
    let listing: Record<string, FakeWindow[]> = {
      'hive-fix-parser': [{ windowId: '@1', name: 'zsh', active: true, width: 80, height: 24 }],
    }
    const listWindows = vi.fn(async (slugs: string[]) =>
      Object.fromEntries(slugs.filter((slug) => listing[slug]).map((slug) => [slug, listing[slug]])),
    )
    mocks.createTerminalClient.mockReturnValue({ listWindows })
    const { wrapper, session } = await mountAvailable()

    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="terminal-window-row"]')).toHaveLength(2)

    // tmux killed the session with its last window, so the live tabs go and
    // the listing behind them is now a listing of nothing.
    listing = {}
    session.tabs.value = []
    session.status.value = 'ended'
    await flushPromises()

    expect(wrapper.findAll('[data-testid="terminal-window-row"]')).toHaveLength(0)
    expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(0)
  })

  it('offers configured agents without launching until one is selected', async () => {
    mocks.SessionLaunchOptions.mockResolvedValue({ agents: ['claude', 'codex', 'fable', 'pi'], defaultAgent: 'claude' })
    const session = fakeSession()
    session.tabs.value.push({ ...session.tabs.value[0], uid: 5, windowId: '@5', name: 'codex' })
    session.activeWindowId.value = '@5'
    mocks.useTerminalWindows.mockReturnValue(session)
    const newAgentWindow = vi.fn().mockResolvedValue({ windowId: '@5' })
    mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows(), newAgentWindow })
    const { wrapper, router } = await mountAt()
    await wrapper.get('[data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="new-window-agent-claude"]').text()).toBe('claude (default)')
    expect(wrapper.get('[data-testid="new-window-menu"]').text()).not.toContain('New agent')
    wrapper.get('[data-testid="new-window-agent-codex"] [data-agent-icon="codex"]')
    wrapper.get('[data-testid="new-window-agent-fable"] [data-agent-icon="claude"]')
    wrapper.get('[data-testid="new-window-agent-pi"] [data-agent-icon="pi"]')
    expect(mocks.SessionLaunchOptions).toHaveBeenCalledOnce()
    await wrapper.get('[data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]').trigger('click')
    await wrapper.get('[data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]').trigger('click')
    await flushPromises()
    expect(mocks.SessionLaunchOptions).toHaveBeenCalledOnce()
    expect(newAgentWindow).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="new-window-agent-codex"]').trigger('click')
    await flushPromises()

    expect(newAgentWindow).toHaveBeenCalledExactlyOnceWith('hive-bump-deps', 'codex')
    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
    expect(router.currentRoute.value.query.window).toBe('@5')
    expect(mocks.openBlank).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="new-window-menu"]').exists()).toBe(false)
  })

  it('adds a peer to the selected session without replacing its existing windows', async () => {
    const newAgentWindow = vi.fn().mockResolvedValue({ windowId: '@5' })
    mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows(), newAgentWindow })
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    const { wrapper } = await mountAt('/terminal/hive-fix-parser')
    await wrapper.get('[data-slug="hive-fix-parser"] [data-testid="terminal-new-window"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="new-window-agent-codex"]').trigger('click')
    await flushPromises()
    expect(session.newAgentWindow).toHaveBeenCalledExactlyOnceWith('codex')
    expect(newAgentWindow).not.toHaveBeenCalled()
    expect(session.newWindow).not.toHaveBeenCalled()
    expect(session.dispose).not.toHaveBeenCalled()
    expect(session.tabs.value).toHaveLength(2)
  })

  it('reports an agent launch failure without opening a shell or changing sessions', async () => {
    const newAgentWindow = vi.fn().mockRejectedValue(new Error('agent launch failed'))
    mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows(), newAgentWindow })
    const session = fakeSession()
    mocks.useTerminalWindows.mockReturnValue(session)
    const { wrapper, router } = await mountAt('/terminal/hive-fix-parser')
    await wrapper.get('[data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="new-window-agent-codex"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('agent launch failed')
    expect(router.currentRoute.value.params.slug).toBe('hive-fix-parser')
    expect(session.newWindow).not.toHaveBeenCalled()
  })

  it('lets agent choices retry loading while retaining the terminal option', async () => {
    mocks.SessionLaunchOptions.mockRejectedValueOnce(new Error('offline'))
    const { wrapper } = await mountAvailable()
    await wrapper.get('[data-slug="hive-fix-parser"] [data-testid="terminal-new-window"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="new-window-terminal"]').exists()).toBe(true)
    await wrapper.get('[data-testid="new-window-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="new-window-agent-codex"]').exists()).toBe(true)
  })

  it('shows a peer launch failure even before any terminal is selected', async () => {
    mocks.createTerminalClient.mockReturnValue({
      listWindows: fakeListWindows(),
      newAgentWindow: vi.fn().mockRejectedValue(new Error('session stopped')),
    })
    const { wrapper } = await mountAt()
    await wrapper.get('[data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="new-window-agent-codex"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="terminal-action-error"]').text()).toBe('session stopped')
  })

  it('disables peer agents when the session is stopped', async () => {
    mocks.SessionStatuses.mockResolvedValue({
      items: [
        { sessionId: '1', running: true, windows: [] },
        { sessionId: '2', running: false, windows: [] },
      ],
      pollIntervalMs: 60_000,
    })
    const { wrapper } = await mountAvailable()
    await wrapper.get('[data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="new-window-agent-codex"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="new-window-terminal"]').text()).toBe('Start session')
  })

  it('adds a window to an unattached session and selects it', async () => {
    const start = vi.fn().mockResolvedValue({ started: true })
    const newWindow = vi.fn(async () => ({ windowId: '@5' }))
    mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows(), start, newWindow })
    const { wrapper, router } = await mountAt()

    await wrapper
      .get('[data-testid="terminal-session-row"][data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]')
      .trigger('click')
    await wrapper.get('[data-testid="new-window-terminal"]').trigger('click')
    await flushPromises()

    expect(newWindow).toHaveBeenCalledWith('hive-bump-deps')
    expect(start).not.toHaveBeenCalled()
    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
  })

  // A stopped session has no window to add to, so + does what the pane's own
  // Start button does instead of asking tmux for a window it does not have.
  it('starts a not-started session instead of asking tmux for a window', async () => {
    const start = vi.fn().mockResolvedValue({ started: true })
    const newWindow = vi.fn(async () => ({ windowId: '@5' }))
    mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows(), start, newWindow })
    mocks.SessionStatuses.mockResolvedValue({
      items: [
        { sessionId: '1', running: true, windows: [] },
        { sessionId: '2', running: false, windows: [] },
      ],
      pollIntervalMs: 60_000,
    })
    const { wrapper, router } = await mountAt()

    await wrapper
      .get('[data-testid="terminal-session-row"][data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]')
      .trigger('click')
    await wrapper.get('[data-testid="new-window-terminal"]').trigger('click')
    await flushPromises()

    expect(start).toHaveBeenCalledWith('hive-bump-deps')
    expect(newWindow).not.toHaveBeenCalled()
    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
  })

  // A start can fail, and its error renders on the row's own pane rather than
  // wherever the user was standing, so the row has to be selected regardless
  // of whether the start that follows succeeds.
  it('still navigates to a not-started session whose start failed', async () => {
    const start = vi.fn().mockRejectedValue(new Error('could not start'))
    const newWindow = vi.fn(async () => ({ windowId: '@5' }))
    mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows(), start, newWindow })
    mocks.SessionStatuses.mockResolvedValue({
      items: [
        { sessionId: '1', running: true, windows: [] },
        { sessionId: '2', running: false, windows: [] },
      ],
      pollIntervalMs: 60_000,
    })
    const { wrapper, router } = await mountAt()

    await wrapper
      .get('[data-testid="terminal-session-row"][data-slug="hive-bump-deps"] [data-testid="terminal-new-window"]')
      .trigger('click')
    await wrapper.get('[data-testid="new-window-terminal"]').trigger('click')
    await flushPromises()

    expect(start).toHaveBeenCalledWith('hive-bump-deps')
    expect(newWindow).not.toHaveBeenCalled()
    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
  })

  // The trailing controls sit inside the row, whose own click attaches or
  // selects; a press on one must not do both.
  it('does not select the row a trailing control was pressed in', async () => {
    const { wrapper, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    session.select.mockClear()
    await wrapper.findAll('[data-testid="terminal-close-window"]')[0].trigger('click')

    expect(session.closeWindow).toHaveBeenCalledWith('@1')
    expect(session.select).not.toHaveBeenCalled()
  })

  it('scopes every pane so the terminal owns its keys', async () => {
    const { wrapper } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    for (const pane of wrapper.findAll('[data-testid="terminal-pane"]')) {
      expect(pane.attributes('data-terminal-input-scope')).toBeDefined()
    }
  })

  it('detaches when the ended overlay closes the session and on unmount', async () => {
    const { wrapper, router, session } = await mountAvailable()
    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()

    session.status.value = 'ended'
    await flushPromises()
    await wrapper.get('[data-testid="terminal-close-session"]').trigger('click')
    await flushPromises()
    expect(session.dispose).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="terminal-no-session"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(true)
    // Closing on purpose forgets the session: the route drops the slug and
    // the next bare entry must not re-attach it.
    expect(router.currentRoute.value.params.slug ?? '').toBe('')
    expect(storedRestore()).toEqual({ slug: '', window: '' })

    await sessionRows(wrapper)[0].trigger('click')
    await flushPromises()
    wrapper.unmount()
    expect(session.dispose).toHaveBeenCalledTimes(2)
  })

  it('keeps sessions that cannot be attached out of the tree', async () => {
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'fix the parser', slug: 'hive-fix-parser', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'old work', slug: 'hive-old-work', repo: 'hay-kot/hive', state: 'recycled' },
      { id: '3', name: 'broken', slug: 'hive-broken', repo: 'hay-kot/hive', state: 'corrupted' },
    ])
    const { wrapper } = await mountAvailable()

    // The tree is the attach surface, and only an active session has a tmux
    // session behind it. Prune is how the other two are dealt with.
    const rows = sessionRows(wrapper)
    expect(rows).toHaveLength(1)
    expect(rows[0].attributes('data-slug')).toBe('hive-fix-parser')
    expect(wrapper.get('[data-testid="terminal-repo-group"]').text()).toContain('1')
  })

  it('reads a session on demand from its row menu', async () => {
    mocks.SessionDetail.mockResolvedValue({
      id: '2',
      name: 'fix the parser',
      slug: 'hive-fix-parser',
      repo: 'hay-kot/hive',
      state: 'active',
      path: '/tmp/hive-fix-parser',
      cloneStrategy: 'full',
      worktreeBranch: '',
      tags: null,
      createdAt: '2026-07-01T00:00:00Z',
      updatedAt: '2026-07-02T00:00:00Z',
    })
    const { wrapper } = await mountAvailable()

    await openRowMenu(wrapper, 'hive-fix-parser')
    await wrapper.get('[data-testid="session-menu-detail"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="session-detail-path"]')?.textContent).toBe('/tmp/hive-fix-parser')
    wrapper.unmount()
  })

  it('confirms a delete against the session\u2019s own risk before starting the job', async () => {
    mocks.SessionRisk.mockResolvedValue({ uncommittedChanges: true, unpushedCommits: true, recycleDeletes: false })
    const { wrapper } = await mountAvailable()

    await openRowMenu(wrapper, 'hive-bump-deps')
    await wrapper.get('[data-testid="session-menu-delete"]').trigger('click')
    await flushPromises()

    const dialog = document.querySelector('[data-testid="session-confirmation"]')
    const dangers = dialog?.querySelectorAll('[data-testid="session-confirmation-details"] [data-tone="danger"]')
    expect(dangers).toHaveLength(2)
    expect(dialog?.textContent).toContain('Uncommitted changes')
    expect(dialog?.textContent).toContain('Unpushed commits')
    expect(mocks.DeleteSession).not.toHaveBeenCalled()

    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-confirm"]')!.click()
    await flushPromises()
    expect(mocks.DeleteSession).toHaveBeenCalledWith('2')
    wrapper.unmount()
  })

  it('recycles from the row menu, behind the same confirmation', async () => {
    const { wrapper } = await mountAvailable()

    await openRowMenu(wrapper, 'hive-bump-deps')
    await wrapper.get('[data-testid="session-menu-recycle"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="session-confirmation"]')?.textContent).toContain('resets its clone')
    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-confirm"]')!.click()
    await flushPromises()
    expect(mocks.RecycleSession).toHaveBeenCalledWith('2')
    wrapper.unmount()
  })

  it('offers to start a session tmux is not running rather than starting it on selection', async () => {
    const start = vi.fn().mockResolvedValue({ started: true })
    mocks.createTerminalClient.mockReturnValue({ start })
    const session = fakeSession()
    session.tabs.value = []
    session.status.value = 'ended'
    session.endReason.value = 'not-started'
    session.error.value = 'session "hive-fix-parser" is not running'
    const { wrapper } = await mountAvailable(session)

    await wrapper.find('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
    await flushPromises()

    // Selecting attaches and nothing more: starting runs the session's agent
    // command, so it waits for the click below.
    expect(session.start).toHaveBeenCalled()
    expect(start).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="terminal-session-not-started"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="terminal-session-ended"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="terminal-window-row"]').exists()).toBe(false)

    await wrapper.get('[data-testid="terminal-start-session"]').trigger('click')
    await flushPromises()

    expect(start).toHaveBeenCalledWith('hive-fix-parser')
    expect(session.reconnect).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('reports a start command failure without leaving the panel', async () => {
    const start = vi
      .fn()
      .mockRejectedValue(
        new Error(
          'tmux session "hive-fix-parser": command not found in window "agent" (status 127)\n\n$ missing-agent\nsh: missing-agent: command not found',
        ),
      )
    mocks.createTerminalClient.mockReturnValue({ start })
    const session = fakeSession()
    session.tabs.value = []
    session.status.value = 'ended'
    session.endReason.value = 'not-started'
    const { wrapper } = await mountAvailable(session)
    await wrapper.find('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="terminal-start-session"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="terminal-start-error-title"]').text()).toBe('Session failed to start')
    expect(wrapper.get('[data-testid="terminal-start-error-output"]').text()).toContain('$ missing-agent')
    expect(wrapper.get('[data-testid="terminal-start-error-output"]').text()).toContain(
      'missing-agent: command not found',
    )
    expect(wrapper.get('[data-testid="terminal-start-error-output"]').text()).toContain('status 127')
    expect(wrapper.find('[data-testid="terminal-start-error-copy"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="terminal-start-session"]').text()).toContain('Try again')
    expect(session.reconnect).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('starts a session from its row menu and attaches to it', async () => {
    const start = vi.fn().mockResolvedValue({ started: true })
    mocks.createTerminalClient.mockReturnValue({ start })
    const { wrapper, router } = await mountAvailable()

    await openRowMenu(wrapper, 'hive-bump-deps')
    await wrapper.get('[data-testid="session-menu-start"]').trigger('click')
    await flushPromises()

    // Started first, then selected: attaching before the spawn would only fail.
    expect(start).toHaveBeenCalledWith('hive-bump-deps')
    expect(router.currentRoute.value.params.slug).toBe('hive-bump-deps')
    wrapper.unmount()
  })

  it('kills the attached terminal behind a confirmation and re-attaches onto the start panel', async () => {
    const kill = vi.fn().mockResolvedValue({ killed: true })
    mocks.createTerminalClient.mockReturnValue({ kill })
    const { wrapper, session } = await mountAvailable()
    await wrapper.find('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
    await flushPromises()

    await openRowMenu(wrapper, 'hive-fix-parser')
    await wrapper.get('[data-testid="session-menu-kill"]').trigger('click')
    await flushPromises()

    // What it kills, and what it leaves, is the whole point of confirming it.
    const dialog = document.querySelector('[data-testid="session-confirmation"]')
    expect(dialog?.textContent).toContain('stopping the agent')
    expect(dialog?.textContent).toContain('checkout and its work are untouched')
    expect(kill).not.toHaveBeenCalled()

    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-confirm"]')!.click()
    await flushPromises()

    expect(kill).toHaveBeenCalledWith('hive-fix-parser')
    // Re-attaching is what turns the dead session into the start panel.
    expect(session.reconnect).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('re-attaches under the new slug when the attached session is renamed', async () => {
    mocks.RenameSession.mockResolvedValue({
      id: '1',
      name: 'parse it',
      slug: 'parse-it',
      repo: 'hay-kot/hive',
      state: 'active',
    })
    const { wrapper, router } = await mountAvailable()
    await wrapper.find('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
    await flushPromises()

    await openRowMenu(wrapper, 'hive-fix-parser')
    await wrapper.get('[data-testid="session-menu-rename"]').trigger('click')
    await flushPromises()

    const input = document.querySelector<HTMLInputElement>('[data-testid="session-rename-input"]')!
    input.value = 'parse it'
    input.dispatchEvent(new Event('input'))
    // The list still holds the old slug at this point, which is what lets the
    // rename recognise the renamed session as the attached one.
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'parse it', slug: 'parse-it', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'bump deps', slug: 'hive-bump-deps', repo: 'hay-kot/hive', state: 'active' },
    ])
    document.querySelector<HTMLButtonElement>('[data-testid="session-rename-save"]')!.click()
    await flushPromises()

    expect(mocks.RenameSession).toHaveBeenCalledWith('1', 'parse it')
    // The slug is the tmux target, so the attach has to follow it.
    expect(router.currentRoute.value.params.slug).toBe('parse-it')
    expect(mocks.useTerminalWindows).toHaveBeenLastCalledWith('parse-it', expect.anything())
    wrapper.unmount()
  })

  it('prunes from the sidebar header, naming how many sessions go', async () => {
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'fix the parser', slug: 'hive-fix-parser', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'old work', slug: 'hive-old-work', repo: 'hay-kot/hive', state: 'recycled' },
      { id: '3', name: 'broken', slug: 'hive-broken', repo: 'hay-kot/hive', state: 'corrupted' },
    ])
    const { wrapper } = await mountAvailable()

    await wrapper.get('[data-testid="terminal-sessions-menu-toggle"]').trigger('click')
    const prune = wrapper.get('[data-testid="terminal-sessions-prune"]')
    expect(prune.text()).toContain('Prune 2 recycled')
    await prune.trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="session-confirmation"]')?.textContent).toContain(
      'All 2 recycled and corrupted sessions',
    )
    document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-confirm"]')!.click()
    await flushPromises()
    expect(mocks.PruneSessions).toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    ['deleted', [{ id: '2', name: 'bump deps', slug: 'hive-bump-deps', repo: 'hay-kot/hive', state: 'active' }]],
    [
      'recycled',
      [
        { id: '1', name: 'fix the parser', slug: 'hive-fix-parser', repo: 'hay-kot/hive', state: 'recycled' },
        { id: '2', name: 'bump deps', slug: 'hive-bump-deps', repo: 'hay-kot/hive', state: 'active' },
      ],
    ],
  ])('closes the attach when the attached session is %s', async (_case, listing) => {
    const { wrapper, router, session } = await mountAvailable()
    await wrapper.find('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
    await flushPromises()

    // Delete and recycle finish as jobs, so a jobs:updated reload is how the
    // frontend learns the session it was attached to can no longer be attached.
    mocks.ListSessions.mockResolvedValue(listing)
    await wrapper.get('[data-testid="terminal-sessions-refresh"]').trigger('click')
    await flushPromises()

    expect(session.dispose).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="terminal-no-session"]').exists()).toBe(true)
    expect(router.currentRoute.value.params.slug ?? '').toBe('')
    expect(storedRestore()).toEqual({ slug: '', window: '' })
  })

  it('lists a session the CLI created on the sessions:updated wake-up', async () => {
    const { wrapper } = await mountAvailable()
    expect(wrapper.find('[data-testid="terminal-session-row"][data-slug="hive-task1"]').exists()).toBe(false)
    const handler = mocks.On.mock.calls.find(([event]) => event === 'sessions:updated')?.[1] as (() => void) | undefined
    expect(handler).toBeDefined()

    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'fix the parser', slug: 'hive-fix-parser', repo: 'hay-kot/hive', state: 'active' },
      { id: '2', name: 'bump deps', slug: 'hive-bump-deps', repo: 'hay-kot/hive', state: 'active' },
      { id: '3', name: 'task1', slug: 'hive-task1', repo: 'hay-kot/hive', state: 'active' },
    ])
    handler!()
    await flushPromises()

    expect(wrapper.find('[data-testid="terminal-session-row"][data-slug="hive-task1"]').exists()).toBe(true)
  })

  // The window well is the only place windows are ordered — a window can only
  // land back in the session it left.
  describe('reordering windows in the sidebar', () => {
    async function attached() {
      const mounted = await mountAvailable()
      await mounted.wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()
      return mounted
    }

    // happy-dom lays nothing out, so the drop edge has to be stated: a row 28px
    // tall at top, and a pointer somewhere in it.
    function box(slot: DOMWrapper<Element>, top: number): void {
      ;(slot.element as HTMLElement).getBoundingClientRect = () =>
        ({ left: 0, width: 220, right: 220, top, bottom: top + 28, height: 28, x: 0, y: top }) as DOMRect
    }

    it('drops a window into the gap the pointer is nearest', async () => {
      const { wrapper, session } = await attached()
      const slots = wrapper.findAll('[data-testid="terminal-window-slot"]')
      box(slots[1], 28)

      await slots[0].trigger('dragstart', { dataTransfer: new DataTransfer() })
      // Past the middle of the second row: after it, which is the end.
      await slots[1].trigger('dragover', { dataTransfer: new DataTransfer(), clientY: 50 })
      expect(wrapper.findAll('[data-testid="terminal-window-slot"]')[1].classes()).toContain('drop-after')

      await slots[1].trigger('drop')
      expect(session.moveWindow).toHaveBeenCalledWith('@1', 1)
    })

    it('marks the gap it would drop into, and clears it when the drag ends', async () => {
      const { wrapper } = await attached()
      const slots = wrapper.findAll('[data-testid="terminal-window-slot"]')
      box(slots[1], 28)

      await slots[0].trigger('dragstart', { dataTransfer: new DataTransfer() })
      await slots[1].trigger('dragover', { dataTransfer: new DataTransfer(), clientY: 30 })
      expect(wrapper.findAll('[data-testid="terminal-window-slot"]')[1].classes()).toContain('drop-before')
      expect(slots[0].classes()).toContain('opacity-40')

      await slots[0].trigger('dragend')
      expect(wrapper.findAll('[data-testid="terminal-window-slot"]')[1].classes()).not.toContain('drop-before')
    })

    // A window dropped where it already is has not moved: both of its own edges
    // name the gap it fills, and reading either as an insertion put it last.
    it('does not move a window dropped back onto itself', async () => {
      const { wrapper, session } = await attached()
      const slots = wrapper.findAll('[data-testid="terminal-window-slot"]')
      box(slots[0], 0)

      await slots[0].trigger('dragstart', { dataTransfer: new DataTransfer() })
      await slots[0].trigger('dragover', { dataTransfer: new DataTransfer(), clientY: 5 })
      await slots[0].trigger('drop')

      expect(session.moveWindow).not.toHaveBeenCalled()
      expect(wrapper.findAll('[data-testid="terminal-window-slot"]')[0].classes()).not.toContain('drop-after')
    })

    // A drag started on the name field would take the row instead of selecting
    // the text in it.
    it('leaves a window being renamed undraggable', async () => {
      const { wrapper } = await attached()
      expect(wrapper.findAll('[data-testid="terminal-window-slot"]')[1].attributes('draggable')).toBe('true')

      await wrapper.findAll('[data-testid="terminal-window-row"]')[1].trigger('dblclick')

      expect(wrapper.findAll('[data-testid="terminal-window-slot"]')[1].attributes('draggable')).toBe('false')
    })

    it('refuses a window dragged into another session', async () => {
      const { wrapper, session } = await attached()
      // Both sessions pooled, so both subtrees carry live rows.
      await wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-bump-deps"]').trigger('click')
      await flushPromises()

      const slots = wrapper.findAll('[data-testid="terminal-window-slot"]')
      expect(slots).toHaveLength(4)
      box(slots[2], 100)

      await slots[0].trigger('dragstart', { dataTransfer: new DataTransfer() })
      await slots[2].trigger('dragover', { dataTransfer: new DataTransfer(), clientY: 120 })
      expect(wrapper.findAll('[data-testid="terminal-window-slot"]')[2].classes()).not.toContain('drop-after')

      await slots[2].trigger('drop')
      expect(session.moveWindow).not.toHaveBeenCalled()
    })

    // Moving a window is a control-client operation, so a session that is only
    // listed has nothing to move it with.
    it('leaves a listed window undraggable', async () => {
      mocks.createTerminalClient.mockReturnValue({
        listWindows: fakeListWindows({
          'hive-bump-deps': [{ windowId: '@7', name: 'agent', active: true, width: 0, height: 0 }],
        }),
      })
      const { wrapper } = await attached()

      const slots = wrapper.findAll('[data-testid="terminal-window-slot"]')
      expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(1)
      expect(slots[0].attributes('draggable')).toBe('false')
      expect(slots[1].attributes('draggable')).toBe('true')
    })
  })

  // Free space: one tmux session that belongs to no piece of work, headed like a
  // repository and listing its tabs where a repository lists its sessions.
  describe('the pinned scratch terminal', () => {
    type Row = Pick<DOMWrapper<Element>, 'text' | 'attributes' | 'find' | 'get' | 'trigger'>
    function heading(wrapper: { get: (s: string) => Row }): Row {
      return wrapper.get('[data-testid="terminal-scratch-heading"]')
    }

    const oneTab = { windowId: '@1', name: 'zsh', active: true, width: 0, height: 0 }

    it('heads the tree with a section of its own, above the repositories', async () => {
      const { wrapper } = await mountAvailable()

      const sections = wrapper.findAll('[data-testid="terminal-scratch-heading"], [data-testid="terminal-repo-group"]')
      expect(sections.map((section) => section.attributes('data-testid'))).toEqual([
        'terminal-scratch-heading',
        'terminal-repo-group',
      ])
      expect(sections[0].text()).toContain('Terminals')
      // The session behind it is a tmux name, not a row: nothing in the tree
      // says Scratch.
      expect(sections[0].text()).not.toContain('Scratch')
      expect(sessionRows(wrapper).map((row) => row.attributes('data-slug'))).not.toContain(SCRATCH_SLUG)
      wrapper.unmount()
    })

    // The tabs take the row a repository gives a session — same box, same
    // controls — rather than hanging two levels in under a row of their own.
    it('lists its tabs where a repository lists its sessions', async () => {
      mocks.createTerminalClient.mockReturnValue({
        listWindows: fakeListWindows({
          [SCRATCH_SLUG]: [oneTab, { windowId: '@2', name: 'nvim', active: false, width: 0, height: 0 }],
        }),
      })
      const { wrapper } = await mountAvailable()

      const tabs = wrapper.findAll('[data-testid="terminal-listed-window-row"]')
      expect(tabs.map((tab) => tab.text())).toEqual(['zsh', 'nvim'])
      expect(tabs[0].classes()).toContain('window-row-flush')
      wrapper.unmount()
    })

    // A heading with nothing under it leaves the keyboard nothing to land on and
    // the mouse nothing but the + to guess at.
    it('offers a row to start from when there is nothing in it', async () => {
      const start = vi.fn().mockResolvedValue({ started: true })
      mocks.createTerminalClient.mockReturnValue({ start, listWindows: fakeListWindows({}) })
      const { wrapper } = await mountAvailable()

      const empty = wrapper.get('[data-testid="terminal-start-scratch"]')
      expect(empty.text()).toBe('Start a terminal')
      // It is a tree row, so the walk stops on it and Enter does what a click does.
      expect(empty.attributes('data-tree-key')).toBe(`s:${SCRATCH_SLUG}`)

      await empty.trigger('click')
      await flushPromises()
      expect(start).toHaveBeenCalledWith(SCRATCH_SLUG)
      wrapper.unmount()
    })

    it('folds its tabs away from the heading, and keeps the heading', async () => {
      mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows({ [SCRATCH_SLUG]: [oneTab] }) })
      const { wrapper } = await mountAvailable()
      expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(1)

      await heading(wrapper).trigger('click')

      expect(heading(wrapper).attributes('aria-expanded')).toBe('false')
      expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]')).toHaveLength(0)
      wrapper.unmount()
    })

    // Its tabs are the section, so they are listed whatever "always show
    // windows" says — that setting is about how much of every session to draw.
    it('lists its tabs with window listing off, and reads its liveness from them', async () => {
      const listWindows = fakeListWindows({ [SCRATCH_SLUG]: [oneTab] })
      mocks.createTerminalClient.mockReturnValue({ listWindows })
      useTerminalShowWindows().setShowWindows(false)
      const { wrapper } = await mountAvailable()

      expect(listWindows).toHaveBeenCalledWith([SCRATCH_SLUG])
      expect(wrapper.findAll('[data-testid="terminal-listed-window-row"]').map((tab) => tab.text())).toEqual(['zsh'])
      expect(heading(wrapper).find('[data-testid="terminal-session-liveness"]').exists()).toBe(true)
      wrapper.unmount()
    })

    // + is the whole affordance on the heading: with tmux holding nothing, the
    // session that gets created *is* the first tab, so there is no start to have
    // missed and no attach to wait for.
    it('starts on + while nothing is running, and adds a tab once something is', async () => {
      const start = vi.fn().mockResolvedValue({ started: true })
      const newWindow = vi.fn().mockResolvedValue({ windowId: '@2' })
      mocks.createTerminalClient.mockReturnValue({ start, newWindow, listWindows: fakeListWindows({}) })
      const { wrapper, session } = await mountAvailable()

      await heading(wrapper).get('[data-testid="terminal-new-window"]').trigger('click')
      await flushPromises()
      expect(start).toHaveBeenCalledWith(SCRATCH_SLUG)
      expect(newWindow).not.toHaveBeenCalled()

      // The same press once it is attached adds a tab through its live client.
      await heading(wrapper).get('[data-testid="terminal-new-window"]').trigger('click')
      await flushPromises()
      expect(session.newWindow).toHaveBeenCalled()
      wrapper.unmount()
    })

    // What the user hits first: tmux is holding the session from a previous run,
    // nothing in this window has attached to it yet, and + has to work anyway.
    it('adds a tab to a running terminal it has not attached to', async () => {
      const start = vi.fn().mockResolvedValue({ started: false })
      const newWindow = vi.fn().mockResolvedValue({ windowId: '@2' })
      mocks.createTerminalClient.mockReturnValue({
        start,
        newWindow,
        listWindows: fakeListWindows({ [SCRATCH_SLUG]: [oneTab] }),
      })
      const { wrapper } = await mountAvailable()

      await heading(wrapper).get('[data-testid="terminal-new-window"]').trigger('click')
      await flushPromises()

      // Starting answers "already running", and the window is made on the slug —
      // the core serves it without this window having to attach first.
      expect(newWindow).toHaveBeenCalledWith(SCRATCH_SLUG)
      wrapper.unmount()
    })

    it('opens a tab by clicking it, and keeps its attach across a session-list reload', async () => {
      mocks.createTerminalClient.mockReturnValue({ listWindows: fakeListWindows({ [SCRATCH_SLUG]: [oneTab] }) })
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper, router } = await mountAt()

      await wrapper.get('[data-testid="terminal-listed-window-row"]').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.params.slug).toBe(SCRATCH_SLUG)
      expect(mocks.useTerminalWindows).toHaveBeenCalledWith(SCRATCH_SLUG, expect.anything())

      // The listing is hive's and carries no scratch terminal, so a reload that
      // drops every pooled session it no longer lists must not take this one.
      await wrapper.get('[data-testid="terminal-sessions-refresh"]').trigger('click')
      await flushPromises()
      expect(session.dispose).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    it('says where it opens when there is no terminal behind it', async () => {
      const start = vi.fn().mockResolvedValue({ started: true })
      mocks.createTerminalClient.mockReturnValue({ start, listWindows: fakeListWindows({}) })
      const session = fakeSession()
      session.tabs.value = []
      session.status.value = 'ended'
      session.endReason.value = 'not-started'
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt(`/terminal/${SCRATCH_SLUG}`)

      const panel = wrapper.get('[data-testid="terminal-session-not-started"]')
      expect(panel.text()).toContain('opens a shell in your home directory')
      expect(panel.text()).not.toContain('agent command')

      await wrapper.get('[data-testid="terminal-start-session"]').trigger('click')
      await flushPromises()
      expect(start).toHaveBeenCalledWith(SCRATCH_SLUG)
      wrapper.unmount()
    })

    // Rename, recycle, delete and details all address a hive session, and there
    // is none behind this section — so are the configured actions, which render
    // over one.
    it('offers only the terminal’s own lifecycle in its menu', async () => {
      mocks.TerminalActionViews.mockResolvedValue([
        { id: 'open-in-zed', label: 'Open in Zed', type: 'shell', showInDetail: false, requiresSessionInput: false },
      ])
      const { wrapper } = await mountAvailable()

      await heading(wrapper).get('[data-testid="terminal-session-menu-toggle"]').trigger('click')

      expect(wrapper.find('[data-testid="session-menu-start"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="session-menu-kill"]').exists()).toBe(true)
      for (const entry of ['detail', 'rename', 'recycle', 'delete']) {
        expect(wrapper.find(`[data-testid="session-menu-${entry}"]`).exists()).toBe(false)
      }
      expect(wrapper.find('[data-testid="terminal-action-open-in-zed"]').exists()).toBe(false)
      wrapper.unmount()
    })

    it('names the tabs rather than a checkout when killed', async () => {
      mocks.createTerminalClient.mockReturnValue({ kill: vi.fn().mockResolvedValue({ killed: true }) })
      const { wrapper } = await mountAvailable()

      await heading(wrapper).get('[data-testid="terminal-session-menu-toggle"]').trigger('click')
      await wrapper.get('[data-testid="session-menu-kill"]').trigger('click')
      await flushPromises()

      const dialog = document.querySelector('[data-testid="session-confirmation"]')
      expect(dialog?.textContent).toContain('Every tab in the scratch terminal is closed')
      expect(dialog?.textContent).not.toContain('checkout')
      document.querySelector<HTMLButtonElement>('[data-testid="session-confirmation-cancel"]')?.click()
      wrapper.unmount()
    })
  })

  describe('pinned agent chats', () => {
    const CHAT_SLUG = 'agentws-7'
    const chat = {
      id: '7',
      workspace: 'demo',
      name: 'api-refactor',
      agent: 'claude',
      lastOpenedAt: 0,
      slug: CHAT_SLUG,
      terminalId: CHAT_SLUG,
      windowId: '@1',
      cols: 80,
      rows: 24,
      resumeAttempted: true,
      notice: '',
    }

    // The listing has to have landed before a pin resolves to a row, so every
    // test here pins after the mount rather than seeding localStorage.
    async function mountWithPinnedChat(session = fakeSession(), listed: typeof chat = chat) {
      mocks.AgentsAvailable.mockResolvedValue({ available: true, reason: '' })
      mocks.allSessions.mockResolvedValue([listed])
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper, router } = await mountAt()
      useTerminalPinnedChats().togglePin(listed.id)
      await flushPromises()
      return { wrapper, router, session }
    }

    function chatRows(wrapper: { findAll: (s: string) => DOMWrapper<Element>[] }): DOMWrapper<Element>[] {
      return wrapper.findAll('[data-testid="terminal-chat-row"]')
    }

    it('heads the tree with a Chats section, above the scratch terminal and the repositories', async () => {
      const { wrapper } = await mountWithPinnedChat()

      const sections = wrapper.findAll(
        '[data-testid="terminal-chats-group"], [data-testid="terminal-scratch-heading"], [data-testid="terminal-repo-group"]',
      )
      expect(sections.map((section) => section.attributes('data-testid'))).toEqual([
        'terminal-chats-group',
        'terminal-scratch-heading',
        'terminal-repo-group',
      ])
      expect(sections[0].text()).toContain('Chats')
      expect(chatRows(wrapper).map((row) => row.text())).toContain('api-refactor')
      wrapper.unmount()
    })

    it('draws no section at all when nothing is pinned', async () => {
      mocks.AgentsAvailable.mockResolvedValue({ available: true, reason: '' })
      mocks.allSessions.mockResolvedValue([chat])
      const { wrapper } = await mountAvailable()

      expect(wrapper.find('[data-testid="terminal-chats-group"]').exists()).toBe(false)
      expect(chatRows(wrapper)).toHaveLength(0)
      wrapper.unmount()
    })

    // The chat's tmux session is addressed exactly like a hive slug, so the pane
    // it opens in is the Code view's own — not a trip to the Agents area.
    it('attaches the chat in the Code view’s pane when its row is picked', async () => {
      const { wrapper, router } = await mountWithPinnedChat()

      await chatRows(wrapper)[0].trigger('click')
      await flushPromises()

      expect(router.currentRoute.value.params.slug).toBe(CHAT_SLUG)
      expect(mocks.useTerminalWindows).toHaveBeenCalledWith(CHAT_SLUG, expect.anything())
      wrapper.unmount()
    })

    // A chat is one conversation; its tmux window is how that is carried rather
    // than something to navigate between.
    it('lists no windows under a chat', async () => {
      mocks.createTerminalClient.mockReturnValue({
        listWindows: fakeListWindows({
          [CHAT_SLUG]: [{ windowId: '@1', name: 'claude', active: true, width: 0, height: 0 }],
        }),
      })
      const { wrapper } = await mountWithPinnedChat()

      const rows = wrapper.findAll('[data-testid="terminal-listed-window-row"], [data-testid="terminal-window-row"]')
      expect(rows.map((row) => row.text())).not.toContain('claude')
      wrapper.unmount()
    })

    // The sweep answers whether tmux is holding the chat, the way it does for the
    // scratch terminal: hive's status projection has no row for either.
    it('reads its liveness off the window sweep rather than hive’s statuses', async () => {
      mocks.createTerminalClient.mockReturnValue({
        listWindows: fakeListWindows({
          [CHAT_SLUG]: [{ windowId: '@1', name: 'claude', active: true, width: 0, height: 0 }],
        }),
      })
      const { wrapper } = await mountWithPinnedChat()

      expect(chatRows(wrapper)[0].find('[data-testid="terminal-session-liveness"]').exists()).toBe(true)
      wrapper.unmount()
    })

    it('unpins from its own row menu, which drops the row', async () => {
      const { wrapper } = await mountWithPinnedChat()

      await wrapper
        .get(`[data-testid="terminal-chat-row"][data-slug="${CHAT_SLUG}"] [data-testid="terminal-chat-menu-toggle"]`)
        .trigger('click')
      await wrapper.get('[data-testid="terminal-chat-unpin"]').trigger('click')
      await flushPromises()

      expect(chatRows(wrapper)).toHaveLength(0)
      expect(wrapper.find('[data-testid="terminal-chats-group"]').exists()).toBe(false)
      wrapper.unmount()
    })

    // Rename, recycle, delete and details all address a hive session, and a chat
    // has none: its record is the Agents area's.
    it('offers only the pin’s own two entries in its menu', async () => {
      const { wrapper } = await mountWithPinnedChat()

      await wrapper
        .get(`[data-testid="terminal-chat-row"][data-slug="${CHAT_SLUG}"] [data-testid="terminal-chat-menu-toggle"]`)
        .trigger('click')

      expect(wrapper.find('[data-testid="terminal-chat-open-in-agents"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="terminal-chat-unpin"]').exists()).toBe(true)
      for (const entry of ['start', 'kill', 'detail', 'rename', 'recycle', 'delete']) {
        expect(wrapper.find(`[data-testid="session-menu-${entry}"]`).exists()).toBe(false)
      }
      wrapper.unmount()
    })

    it('routes back to the Agents area with the chat open', async () => {
      const { wrapper, router } = await mountWithPinnedChat()

      await wrapper
        .get(`[data-testid="terminal-chat-row"][data-slug="${CHAT_SLUG}"] [data-testid="terminal-chat-menu-toggle"]`)
        .trigger('click')
      await wrapper.get('[data-testid="terminal-chat-open-in-agents"]').trigger('click')
      await flushPromises()

      expect(router.currentRoute.value.name).toBe('agents')
      expect(router.currentRoute.value.params.workspace).toBe('demo')
      expect(router.currentRoute.value.query.chat).toBe('7')
      wrapper.unmount()
    })

    // There is no hive session behind an agentws-* slug for a spawn
    // configuration to be read from, so starting one has to be the Agents area's
    // resume — and it stays an offered action rather than something the attach does.
    it('resumes a stopped chat through the Agents area rather than hive’s start', async () => {
      const start = vi.fn().mockResolvedValue({ started: true })
      mocks.createTerminalClient.mockReturnValue({ start, listWindows: fakeListWindows({}) })
      mocks.resumeSession.mockResolvedValue({ ...chat, terminalId: CHAT_SLUG })
      const session = fakeSession()
      session.tabs.value = []
      session.status.value = 'ended'
      session.endReason.value = 'not-started'
      const { wrapper } = await mountWithPinnedChat(session, {
        ...chat,
        terminalId: '',
        windowId: '',
        cols: 0,
        rows: 0,
      })

      await chatRows(wrapper)[0].trigger('click')
      await flushPromises()

      const panel = wrapper.get('[data-testid="terminal-session-not-started"]')
      expect(panel.text()).toContain('Chat not running')
      expect(panel.text()).not.toContain('agent command')
      expect(wrapper.get('[data-testid="terminal-start-session"]').text()).toContain('Resume chat')

      await wrapper.get('[data-testid="terminal-start-session"]').trigger('click')
      await flushPromises()

      expect(mocks.resumeSession).toHaveBeenCalledWith({ id: '7' })
      expect(start).not.toHaveBeenCalled()
      wrapper.unmount()
    })
  })

  describe('configured terminal actions', () => {
    const openInZed = {
      id: 'open-in-zed',
      label: 'Open in Zed',
      type: 'shell',
      showInDetail: false,
      requiresSessionInput: false,
    }
    const interrupt = {
      id: 'interrupt',
      label: 'Interrupt',
      type: 'shell',
      showInDetail: false,
      requiresSessionInput: false,
    }

    function offer(session: unknown[], window: unknown[] = []) {
      mocks.TerminalActionViews.mockImplementation(async (target: string) => (target === 'session' ? session : window))
    }

    it('offers session-targeted actions in the session row menu and runs one against its slug', async () => {
      offer([openInZed])
      const { wrapper } = await mountAvailable()

      await wrapper
        .get(
          '[data-testid="terminal-session-row"][data-slug="hive-fix-parser"] [data-testid="terminal-session-menu-toggle"]',
        )
        .trigger('click')
      await wrapper.get('[data-testid="terminal-action-open-in-zed"]').trigger('click')
      await flushPromises()

      expect(mocks.InvokeTerminalAction).toHaveBeenCalledWith(
        'open-in-zed',
        { slug: 'hive-fix-parser', windowId: '' },
        {},
      )
      wrapper.unmount()
    })

    it('gives a window row its own menu, carrying the window id', async () => {
      offer([], [interrupt])
      const { wrapper } = await mountAvailable()
      await wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()

      const window = wrapper.get('[data-testid="terminal-window-row"][data-window-id="@2"]')
      await window.get('[data-testid="terminal-window-menu-toggle"]').trigger('click')
      await window.get('[data-testid="terminal-action-interrupt"]').trigger('click')
      await flushPromises()

      expect(mocks.InvokeTerminalAction).toHaveBeenCalledWith(
        'interrupt',
        { slug: 'hive-fix-parser', windowId: '@2' },
        {},
      )
      wrapper.unmount()
    })

    // A window row has no operations of its own, so an empty menu would be an
    // affordance that does nothing.
    it('grows no window menu when nothing targets a window', async () => {
      offer([openInZed])
      const { wrapper } = await mountAvailable()
      await wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()

      expect(wrapper.find('[data-testid="terminal-window-menu-toggle"]').exists()).toBe(false)
      wrapper.unmount()
    })

    it('collects declared inputs before running', async () => {
      offer([
        {
          ...openInZed,
          inputs: [
            {
              name: 'branch',
              label: 'Branch',
              type: 'text',
              required: true,
              default: '',
              placeholder: '',
              options: [],
            },
          ],
        },
      ])
      const { wrapper } = await mountAvailable()

      await wrapper
        .get(
          '[data-testid="terminal-session-row"][data-slug="hive-fix-parser"] [data-testid="terminal-session-menu-toggle"]',
        )
        .trigger('click')
      await wrapper.get('[data-testid="terminal-action-open-in-zed"]').trigger('click')
      await flushPromises()
      expect(mocks.InvokeTerminalAction).not.toHaveBeenCalled()

      const field = document.querySelector<HTMLInputElement>('[data-testid="action-inputs-dialog"] input')!
      field.value = 'main'
      field.dispatchEvent(new Event('input'))
      await flushPromises()
      document.querySelector<HTMLButtonElement>('[data-testid="action-inputs-submit"]')!.click()
      await flushPromises()

      expect(mocks.InvokeTerminalAction).toHaveBeenCalledWith(
        'open-in-zed',
        { slug: 'hive-fix-parser', windowId: '' },
        { branch: 'main' },
      )
      wrapper.unmount()
    })

    // Text is not a side effect: a clipboard action renders and copies rather
    // than starting a job, the same split the detail pane makes.
    it('copies a clipboard action instead of running it', async () => {
      offer([
        { id: 'copy-path', label: 'Copy path', type: 'clipboard', showInDetail: false, requiresSessionInput: false },
      ])
      mocks.RenderTerminalClipboardAction.mockResolvedValue('/work/hive-fix-parser')
      const { wrapper } = await mountAvailable()

      await wrapper
        .get(
          '[data-testid="terminal-session-row"][data-slug="hive-fix-parser"] [data-testid="terminal-session-menu-toggle"]',
        )
        .trigger('click')
      await wrapper.get('[data-testid="terminal-action-copy-path"]').trigger('click')
      await flushPromises()

      expect(mocks.RenderTerminalClipboardAction).toHaveBeenCalledWith(
        'copy-path',
        { slug: 'hive-fix-parser', windowId: '' },
        {},
      )
      expect(mocks.SetClipboardText).toHaveBeenCalledWith('/work/hive-fix-parser')
      expect(mocks.InvokeTerminalAction).not.toHaveBeenCalled()
      wrapper.unmount()
    })
  })

  describe('filtering the session list', () => {
    async function filter(wrapper: { get: (s: string) => Pick<DOMWrapper<Element>, 'setValue'> }, query: string) {
      await wrapper.get('[data-testid="terminal-sessions-filter"]').setValue(query)
      await flushPromises()
    }

    // The pinned scratch row is filtered like any other and has a test of its
    // own below; these read the repositories.
    function slugs(wrapper: { findAll: (s: string) => DOMWrapper<Element>[] }): (string | undefined)[] {
      return sessionRows(wrapper).map((row) => row.attributes('data-slug'))
    }

    it('narrows the tree to the sessions that match, and says when none do', async () => {
      const { wrapper } = await mountAvailable()

      await filter(wrapper, 'bump')
      expect(slugs(wrapper)).toEqual(['hive-bump-deps'])

      await filter(wrapper, 'PARSER')
      expect(slugs(wrapper)).toEqual(['hive-fix-parser'])

      await filter(wrapper, 'nothing here')
      expect(slugs(wrapper)).toEqual([])
      expect(wrapper.get('[data-testid="terminal-sessions-no-matches"]').text()).toContain('nothing here')
      wrapper.unmount()
    })

    it('carries a whole repo when the repo itself matches', async () => {
      mocks.ListSessions.mockResolvedValue([
        { id: '1', name: 'fix the parser', slug: 'hive-fix-parser', repo: 'hay-kot/hive', state: 'active' },
        { id: '3', name: 'landing copy', slug: 'site-landing-copy', repo: 'hay-kot/haykot.dev', state: 'active' },
      ])
      const { wrapper } = await mountAvailable()

      await filter(wrapper, 'haykot.dev')
      expect(slugs(wrapper)).toEqual(['site-landing-copy'])
      wrapper.unmount()
    })

    // A collapsed group would hide the very row the query asked for.
    it('opens a collapsed group for as long as the filter is on', async () => {
      const { wrapper } = await mountAvailable()
      await wrapper.get('[data-testid="terminal-repo-group"]').trigger('click')
      expect(slugs(wrapper)).toEqual([])

      await filter(wrapper, 'bump')
      expect(slugs(wrapper)).toEqual(['hive-bump-deps'])

      await filter(wrapper, '')
      expect(slugs(wrapper)).toEqual([])
      wrapper.unmount()
    })

    // Filtering is a way to look at the list, not a way to detach: the session
    // on screen is the one the user is working in.
    it('keeps the attached session on screen while it is filtered out', async () => {
      const { wrapper, router } = await mountAvailable()
      await sessionRows(wrapper)[0].trigger('click')
      await flushPromises()

      await filter(wrapper, 'parser')

      expect(slugs(wrapper)).toEqual(['hive-fix-parser'])
      expect(router.currentRoute.value.path).toBe('/terminal/hive-bump-deps')
      expect(shownWindow(wrapper)).toBe('@1')
      wrapper.unmount()
    })

    it('walks only the rows the filter left', async () => {
      const { wrapper, router } = await mountAvailable()

      await filter(wrapper, 'parser')
      await wrapper.get('[data-testid="terminal-session-sidebar"]').trigger('keydown', { key: 'ArrowDown' })
      await flushPromises()

      expect(router.currentRoute.value.path).toBe('/terminal/hive-fix-parser')
      wrapper.unmount()
    })

    // Esc is the way back out, and it is two keystrokes on purpose: the first
    // restores the list, the second leaves a field that has nothing left to
    // undo. ↓ and Enter are the way out that keeps the query.
    it('clears on Esc, then hands focus to the tree', async () => {
      const { wrapper } = await mountAvailable()
      const field = wrapper.get('[data-testid="terminal-sessions-filter"]')
      // Nothing in this DOM can hold focus, so the move is observed on the call.
      const focusRow = vi.spyOn(HTMLElement.prototype, 'focus').mockImplementation(() => {})

      await filter(wrapper, 'bump')
      await field.trigger('keydown', { key: 'Escape' })
      await flushPromises()
      expect(slugs(wrapper)).toEqual(['hive-bump-deps', 'hive-fix-parser'])
      expect(focusRow).not.toHaveBeenCalled()

      await field.trigger('keydown', { key: 'Escape' })
      await flushPromises()
      expect(focusRow).toHaveBeenCalled()

      focusRow.mockClear()
      await filter(wrapper, 'bump')
      await field.trigger('keydown', { key: 'ArrowDown' })
      await flushPromises()
      expect(focusRow).toHaveBeenCalled()
      expect(slugs(wrapper)).toEqual(['hive-bump-deps'])

      focusRow.mockRestore()
      wrapper.unmount()
    })

    // `/` is dispatched by App.vue against the published handle; this is the
    // half that lands in the field.
    it('takes the field on the focus-filter command, selecting what is there', async () => {
      const { wrapper } = await mountAvailable()
      const input = wrapper.get('[data-testid="terminal-sessions-filter"]').element as HTMLInputElement
      const select = vi.spyOn(input, 'select')

      focusTerminalFilter()
      await flushPromises()

      expect(select).toHaveBeenCalled()
      wrapper.unmount()
    })
  })

  // The other half of the narrowing: the query picks a session by name, this
  // picks by whether tmux is holding one.
  describe('narrowing the tree to what is running', () => {
    const TWO_REPOS = [
      { id: '1', name: 'fix the parser', slug: 'hive-fix-parser', repo: 'hay-kot/hive', state: 'active' },
      { id: '3', name: 'landing copy', slug: 'site-landing-copy', repo: 'hay-kot/haykot.dev', state: 'active' },
    ]

    function running(byID: Record<string, boolean>) {
      mocks.SessionStatuses.mockResolvedValue({
        items: Object.entries(byID).map(([sessionId, isRunning]) => ({ sessionId, running: isRunning, windows: [] })),
        pollIntervalMs: 60_000,
      })
    }

    function slugs(wrapper: { findAll: (s: string) => DOMWrapper<Element>[] }): (string | undefined)[] {
      return sessionRows(wrapper).map((row) => row.attributes('data-slug'))
    }

    function repos(wrapper: { findAll: (s: string) => DOMWrapper<Element>[] }): DOMWrapper<Element>[] {
      return wrapper.findAll('[data-testid="terminal-repo-group"]')
    }

    async function pick(wrapper: { get: (s: string) => Pick<DOMWrapper<Element>, 'trigger'> }, testid: string) {
      await wrapper.get('[data-testid="terminal-sessions-menu-toggle"]').trigger('click')
      await wrapper.get(`[data-testid="${testid}"]`).trigger('click')
      await flushPromises()
    }

    // The repo header is the noise a dormant repository contributes, so it goes
    // with its sessions rather than staying behind as an empty heading.
    it('drops a repository with nothing running, and brings it back on Show all', async () => {
      mocks.ListSessions.mockResolvedValue(TWO_REPOS)
      running({ 1: true, 3: false })
      const { wrapper } = await mountAvailable()
      expect(repos(wrapper)).toHaveLength(2)

      await pick(wrapper, 'terminal-sessions-running-only')
      expect(repos(wrapper).map((group) => group.attributes('data-repo'))).toEqual(['hay-kot/hive'])
      expect(slugs(wrapper)).toEqual(['hive-fix-parser'])

      await wrapper.get('[data-testid="terminal-sessions-running-clear"]').trigger('click')
      expect(repos(wrapper).map((group) => group.attributes('data-repo'))).toEqual([
        'hay-kot/haykot.dev',
        'hay-kot/hive',
      ])
      wrapper.unmount()
    })

    // Same rule the query follows: narrowing is a way to look at the list, not
    // a way to detach.
    it('keeps the attached session on screen while the filter hides it', async () => {
      running({ 1: true, 2: false })
      const { wrapper, router } = await mountAvailable()
      await sessionRows(wrapper)[0].trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.path).toBe('/terminal/hive-bump-deps')

      await pick(wrapper, 'terminal-sessions-running-only')

      expect(slugs(wrapper)).toEqual(['hive-fix-parser'])
      expect(router.currentRoute.value.path).toBe('/terminal/hive-bump-deps')
      expect(shownWindow(wrapper)).toBe('@1')
      wrapper.unmount()
    })

    // The pinned section is exempt from the filter, so it cannot stand in as
    // something the filter found — the note has to name the reason the tree is
    // bare, and it is not the (empty) query.
    it('says the tree is bare because nothing is running, not because nothing matched', async () => {
      running({ 1: false, 2: false })
      const { wrapper } = await mountAvailable()

      await pick(wrapper, 'terminal-sessions-running-only')

      expect(slugs(wrapper)).toEqual([])
      expect(wrapper.get('[data-testid="terminal-sessions-no-matches"]').text()).toBe('No sessions are running.')
      wrapper.unmount()
    })

    // A tree that is short because it was narrowed reads exactly like one that
    // is short because the sessions are gone — and the filter outlives the run.
    it('names what it is holding back until it is cleared', async () => {
      mocks.ListSessions.mockResolvedValue(TWO_REPOS)
      running({ 1: true, 3: false })
      const { wrapper } = await mountAvailable()
      expect(wrapper.find('[data-testid="terminal-sessions-running-note"]').exists()).toBe(false)

      await pick(wrapper, 'terminal-sessions-running-only')
      expect(wrapper.get('[data-testid="terminal-sessions-running-note"]').text()).toContain(
        'Running sessions only · 1 hidden',
      )

      await wrapper.get('[data-testid="terminal-sessions-running-clear"]').trigger('click')
      expect(wrapper.find('[data-testid="terminal-sessions-running-note"]').exists()).toBe(false)
      wrapper.unmount()
    })

    it('keeps the filter across a relaunch', async () => {
      const first = await mountAvailable()
      await pick(first.wrapper, 'terminal-sessions-running-only')
      first.wrapper.unmount()

      const { wrapper } = await mountAvailable()
      expect(wrapper.get('[data-testid="terminal-sessions-running-note"]').text()).toContain('Running sessions only')
      expect(slugs(wrapper)).toEqual(['hive-bump-deps', 'hive-fix-parser'])
      wrapper.unmount()
    })

    it('folds every repository group at once, and unfolds them again', async () => {
      mocks.ListSessions.mockResolvedValue(TWO_REPOS)
      const { wrapper } = await mountAvailable()

      await pick(wrapper, 'terminal-sessions-collapse-all')
      expect(repos(wrapper).map((group) => group.attributes('aria-expanded'))).toEqual(['false', 'false'])
      expect(slugs(wrapper)).toEqual([])

      await pick(wrapper, 'terminal-sessions-expand-all')
      expect(slugs(wrapper)).toEqual(['site-landing-copy', 'hive-fix-parser'])
      wrapper.unmount()
    })
  })

  // An arrow is a click on the neighbouring row — there is no cursor running
  // ahead of the selection and no Enter to commit.
  describe('keyboard navigation in the tree', () => {
    // Attached to the sidebar rather than the row, so it answers wherever focus
    // is — including the panel itself before the first arrow.
    async function press(wrapper: { get: (s: string) => Pick<DOMWrapper<Element>, 'trigger'> }, key: string) {
      await wrapper.get('[data-testid="terminal-session-sidebar"]').trigger('keydown', { key })
      await flushPromises()
    }

    function tabStop(wrapper: { findAll: (s: string) => DOMWrapper<Element>[] }): string | undefined {
      return wrapper
        .findAll('[data-tree-key]')
        .find((row) => row.attributes('tabindex') === '0')
        ?.attributes('data-tree-key')
    }

    // The pinned terminal is the tree's first stop — the test below is about
    // that. The rest walk the repositories from a tree without one, because
    // landing on the pinned row attaches it and unfolds its own tabs into the
    // very list a press count is counting.
    async function mountWalkable(session = fakeSession()) {
      mocks.Scratch.mockRejectedValue(new Error('no scratch terminal in this fixture'))
      return mountAvailable(session)
    }

    // Free space is the first thing the keyboard reaches, and one arrow is what
    // it costs to get there.
    it('starts the walk on the pinned scratch terminal', async () => {
      const { wrapper, router } = await mountAvailable()

      await press(wrapper, 'ArrowDown')

      expect(router.currentRoute.value.path).toBe('/terminal/Scratch')
      expect(mocks.useTerminalWindows).toHaveBeenCalledWith('Scratch', expect.anything())
      wrapper.unmount()
    })

    it('attaches the session an arrow lands on, as a click would', async () => {
      const { wrapper, router } = await mountWalkable()
      expect(wrapper.find('[data-testid="terminal-no-session"]').exists()).toBe(true)

      // Alphabetical within the group, so the first row is 'bump deps'.
      await press(wrapper, 'ArrowDown')

      expect(router.currentRoute.value.path).toBe('/terminal/hive-bump-deps')
      wrapper.unmount()
    })

    // Attaching the first session unfolds its two windows into the walk, so the
    // next session sits four rows down rather than one.
    it('walks through the attached session windows on to the next session', async () => {
      const { wrapper, router } = await mountWalkable()

      for (let i = 0; i < 4; i++) await press(wrapper, 'ArrowDown')

      expect(router.currentRoute.value.path).toBe('/terminal/hive-fix-parser')
      wrapper.unmount()
    })

    it('selects a window row the same way', async () => {
      const session = fakeSession()
      const { wrapper } = await mountWalkable(session)

      await press(wrapper, 'ArrowDown')
      // Onto the attached session's second window.
      await press(wrapper, 'ArrowDown')
      await press(wrapper, 'ArrowDown')

      expect(session.select).toHaveBeenCalledWith('@2')
      wrapper.unmount()
    })

    it('moves on j and k as well', async () => {
      const session = fakeSession()
      const { wrapper, router } = await mountWalkable(session)

      await press(wrapper, 'j')
      expect(router.currentRoute.value.path).toBe('/terminal/hive-bump-deps')

      await press(wrapper, 'j')
      expect(tabStop(wrapper)).toBe('w:2:@2')

      await press(wrapper, 'k')
      expect(tabStop(wrapper)).toBe('w:2:@1')
      wrapper.unmount()
    })

    // A running session is its windows. Stopping on its row first changed
    // nothing, so the row leaves the walk once there is a terminal behind it.
    it('skips a running session row and walks its windows', async () => {
      const { wrapper } = await mountWalkable()

      // Nothing is attached yet and no windows are known, so the row is still
      // the only way in.
      await press(wrapper, 'ArrowDown')
      expect(tabStop(wrapper)).toBe('w:2:@1')

      const walked = wrapper
        .findAll('[data-tree-key][tabindex]')
        .filter((row) => row.attributes('tabindex') === '0')
        .map((row) => row.attributes('data-tree-key'))
      expect(walked).not.toContain('s:2')
      wrapper.unmount()
    })

    // The exception that keeps every session reachable: with window listing off,
    // an unattached session has no windows to stand in for its row.
    it('keeps the row of a running session that lists no windows', async () => {
      useTerminalShowWindows().setShowWindows(false)
      const { wrapper, router } = await mountWalkable()

      // The attached session still contributes its own live windows, so the
      // second session's row is three stops down rather than one.
      for (let i = 0; i < 3; i++) await press(wrapper, 'ArrowDown')

      expect(router.currentRoute.value.path).toBe('/terminal/hive-fix-parser')
      wrapper.unmount()
    })

    // A repo header collapses on click, which is not something to do to every
    // group an arrow passes.
    it('skips repo headers', async () => {
      const { wrapper } = await mountWalkable()

      await press(wrapper, 'ArrowUp')

      expect(wrapper.findAll('[data-tree-key]').map((row) => row.attributes('data-tree-key'))).not.toContain(
        'g:hay-kot/hive',
      )
      wrapper.unmount()
    })

    // One tab stop, not one per row: the tree is a single widget, so Tab steps
    // over it and the arrows move within it.
    it('carries a single tab stop that follows the selection', async () => {
      const { wrapper } = await mountWalkable()

      expect(wrapper.findAll('[data-tree-key][tabindex="0"]')).toHaveLength(1)
      expect(tabStop(wrapper)).toBe('s:2')

      for (let i = 0; i < 3; i++) await press(wrapper, 'ArrowDown')

      expect(wrapper.findAll('[data-tree-key][tabindex="0"]')).toHaveLength(1)
      expect(tabStop(wrapper)).toBe('w:1:@1')
      wrapper.unmount()
    })

    it('clamps at the bottom instead of wrapping', async () => {
      const { wrapper, router } = await mountWalkable()

      for (let i = 0; i < 12; i++) await press(wrapper, 'ArrowDown')

      expect(tabStop(wrapper)).toBe('w:1:@2')
      expect(router.currentRoute.value.path).toBe('/terminal/hive-fix-parser')
      wrapper.unmount()
    })

    // Enter on a row the walk only stops at because nothing is running behind
    // it. A click still just opens it — the pane's own Start button is there.
    it('starts a session that has no terminal on Enter', async () => {
      const start = vi.fn().mockResolvedValue(undefined)
      mocks.createTerminalClient.mockReturnValue({ start })
      mocks.SessionStatuses.mockResolvedValue({
        items: [
          { sessionId: '1', running: false, windows: [] },
          { sessionId: '2', running: true, windows: [] },
        ],
        pollIntervalMs: 60_000,
      })
      const { wrapper } = await mountAvailable()

      await wrapper.get('[data-tree-key="s:1"]').trigger('keydown', { key: 'Enter' })
      await flushPromises()

      expect(start).toHaveBeenCalledWith('hive-fix-parser')

      await wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()

      expect(start).toHaveBeenCalledTimes(1)
      wrapper.unmount()
    })

    // Cmd+↓ is not the tree's to take — it resolves through the global keymap.
    it('leaves modified arrows to the keymap', async () => {
      const { wrapper, router } = await mountAvailable()

      await wrapper
        .get('[data-testid="terminal-session-sidebar"]')
        .trigger('keydown', { key: 'ArrowDown', metaKey: true })
      await flushPromises()

      expect(router.currentRoute.value.path).toBe('/terminal')
      wrapper.unmount()
    })

    it('leaves the arrows to a rename input mid-edit', async () => {
      const { wrapper } = await mountAvailable(fakeSession())
      await wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()
      await wrapper.get('[data-testid="terminal-window-row"]').trigger('dblclick')

      const before = tabStop(wrapper)
      await wrapper.get('[data-testid="terminal-rename-input"]').trigger('keydown', { key: 'ArrowDown' })
      await flushPromises()

      expect(tabStop(wrapper)).toBe(before)
      wrapper.unmount()
    })

    // The bug this guards: a pane that grabs focus mid-walk sends the next
    // arrow to tmux instead of the tree.
    it('keeps focus in the tree while the arrows walk it', async () => {
      const session = fakeSession()
      const { wrapper } = await mountWalkable(session)

      for (let i = 0; i < 3; i++) await press(wrapper, 'ArrowDown')

      expect(paneMayAutoFocus.value).toBe(false)
      expect(session.focusActive).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    // The mouse keeps the old behaviour: clicking a session is how you go to
    // work in it.
    it('lets a click hand focus to the pane', async () => {
      const session = fakeSession()
      const { wrapper } = await mountWalkable(session)

      await press(wrapper, 'ArrowDown')
      expect(paneMayAutoFocus.value).toBe(false)

      await wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()

      expect(paneMayAutoFocus.value).toBe(true)
      wrapper.unmount()
    })

    it('enters the pane on Enter', async () => {
      const session = fakeSession()
      const { wrapper } = await mountWalkable(session)

      await press(wrapper, 'ArrowDown')
      await wrapper.get('[data-tree-key="s:2"]').trigger('keydown', { key: 'Enter' })
      await flushPromises()

      expect(paneMayAutoFocus.value).toBe(true)
      expect(session.focusActive).toHaveBeenCalled()
      wrapper.unmount()
    })

    it('shows the tree keys once there is a tree to drive', async () => {
      const { wrapper } = await mountAvailable()

      const hints = wrapper.get('[data-testid="terminal-tree-hints"]').text()
      expect(hints).toContain('switch')
      expect(hints).toContain('enter')
      wrapper.unmount()
    })

    // The chord worth showing is the one that leaves where focus already is.
    it('offers the way out of whichever side holds focus', async () => {
      const { wrapper } = await mountAvailable()
      const sidebar = wrapper.get('[data-testid="terminal-session-sidebar"]')

      expect(wrapper.get('[data-testid="terminal-tree-hints"]').text()).toContain('tree')

      await sidebar.trigger('focusin')
      expect(wrapper.get('[data-testid="terminal-tree-hints"]').text()).toContain('terminal')

      await sidebar.trigger('focusout')
      expect(wrapper.get('[data-testid="terminal-tree-hints"]').text()).toContain('tree')
      wrapper.unmount()
    })

    // The bar is a legend for a walk, so it goes only when there is nothing to
    // walk — and the pinned scratch row is something, whether or not hive has a
    // session to list.
    it('omits the hint bar when the tree has nothing in it at all', async () => {
      mocks.ListSessions.mockResolvedValue([])
      mocks.Scratch.mockRejectedValue(new Error('the terminal is unavailable'))
      const { wrapper } = await mountAvailable()

      expect(wrapper.find('[data-testid="terminal-tree-hints"]').exists()).toBe(false)
      wrapper.unmount()
    })

    it('keeps the hint bar for the scratch terminal alone', async () => {
      mocks.ListSessions.mockResolvedValue([])
      const { wrapper } = await mountAvailable()

      expect(wrapper.find('[data-testid="terminal-tree-hints"]').exists()).toBe(true)
      wrapper.unmount()
    })
  })

  // ── Attached-windows projection ──────────────────────────────────────────────
  // useAppPaletteRows reads useAttachedTerminalWindows rather than this
  // component's own state, so a window row exists before TerminalMode has ever
  // mounted — this is the write side of that seam.

  describe('attached windows projection', () => {
    it('writes the attached session and its windows when a session attaches, and clears it on unmount', async () => {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt('/terminal/hive-fix-parser')

      const { attached } = useAttachedTerminalWindows()
      expect(attached.value).toEqual({
        slug: 'hive-fix-parser',
        name: 'fix the parser',
        windows: [
          { windowId: '@1', name: 'agent', active: true },
          { windowId: '@2', name: 'shell', active: false },
        ],
      })

      wrapper.unmount()
      expect(attached.value).toBeNull()
    })

    it('writes nothing while no session is attached', async () => {
      const { wrapper } = await mountAt()

      expect(useAttachedTerminalWindows().attached.value).toBeNull()
      wrapper.unmount()
    })

    // Finding: this is the whole point of registering the window rows at App
    // level — a jump has to work from the hub, not only from inside Code. The
    // route clears activeSlug on the way out, so the projection has to be
    // keyed off something the trip does not touch: the warm pool entry.
    it('survives a trip back to the hub', async () => {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { router } = await mountAt('/terminal/hive-fix-parser')
      await flushPromises()

      await router.push({ name: 'feed' })
      await flushPromises()

      expect(useAttachedTerminalWindows().attached.value).toEqual({
        slug: 'hive-fix-parser',
        name: 'fix the parser',
        windows: [
          { windowId: '@1', name: 'agent', active: true },
          { windowId: '@2', name: 'shell', active: false },
        ],
      })
    })

    // The survival above is not "never clears" — it clears once the session
    // the pool is warming genuinely goes away, whether or not Code is on
    // screen when that happens.
    it('clears once the session is actually gone, even away from /terminal', async () => {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { router } = await mountAt('/terminal/hive-fix-parser')
      await flushPromises()
      await router.push({ name: 'feed' })
      await flushPromises()
      expect(useAttachedTerminalWindows().attached.value).not.toBeNull()

      // Deleted (or recycled) elsewhere; the next reload drops it from the
      // listing, and the same pool cleanup that runs in mode lets it go.
      mocks.ListSessions.mockResolvedValue([
        { id: '2', name: 'bump deps', slug: 'hive-bump-deps', repo: 'hay-kot/hive', state: 'active' },
      ])
      await useTerminalSessions().reload()
      await flushPromises()

      expect(session.dispose).toHaveBeenCalled()
      expect(useAttachedTerminalWindows().attached.value).toBeNull()
    })
  })

  // ── Command palette library ────────────────────────────────────────────────
  // The attached session's own operations, under the session's own name.
  // Window and attach rows are App-level now (useAppPaletteRows): they name
  // objects a fresh launch must list before this async component has ever
  // mounted.

  describe('command palette library', () => {
    function paletteResults() {
      const palette = useCommandPalette()
      palette.query.value = ''
      palette.scope.value = 'all'
      return palette.results
    }

    // The user's ask: `!<cmd>` is a quick way to get a shell running that
    // command in the session's own context — a window in the strip, not an
    // ephemeral pop-up, so it outlives the command and can be returned to.
    it('opens a window on the attached session for a !-query and types the line into it', async () => {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt('/terminal/hive-fix-parser')
      const palette = useCommandPalette()

      palette.setQuery('!npm test')
      expect(palette.scope.value).toBe('shell')
      expect(palette.results.value.map((cmd) => cmd.id)).toEqual(['shell:run'])
      expect(palette.results.value[0].title).toBe('Run: npm test')
      expect(palette.results.value[0].hint).toBe('new window in fix the parser')

      void palette.results.value[0].run()
      expect(session.newWindow).toHaveBeenCalledWith('npm test')

      // A bare ! has nothing to run.
      palette.setQuery('')
      palette.setQuery('!')
      expect(palette.results.value).toEqual([])

      palette.query.value = ''
      palette.scope.value = 'all'
      wrapper.unmount()
    })

    // Nothing attached is the session picker: there is no session for the line
    // to run in, so the escape declines rather than guessing one.
    it('offers no shell escape while no session is attached', async () => {
      const { wrapper } = await mountAvailable()
      const palette = useCommandPalette()

      palette.setQuery('!npm test')
      expect(palette.results.value).toEqual([])

      palette.query.value = ''
      palette.scope.value = 'all'
      wrapper.unmount()
    })

    // Window and attach rows register at App level (useAppPaletteRows), so
    // this component's own library is only the attached session's operations.
    it("lists the attached session's operations, under the session's own name", async () => {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt('/terminal/hive-fix-parser')
      const results = paletteResults()

      const byId = new Map(results.value.map((cmd) => [cmd.id, cmd]))
      for (const id of [
        'terminal:session:kill',
        'terminal:session:detail',
        'terminal:session:rename',
        'terminal:session:recycle',
        'terminal:session:delete',
      ]) {
        expect(byId.has(id), id).toBe(true)
        expect(byId.get(id)?.group).toBe('fix the parser')
        expect(byId.get(id)?.scope).toBe('actions')
      }
      // Running already — nothing to start.
      expect(byId.has('terminal:session:start')).toBe(false)

      // No window or attach rows — those register at App level (the fake
      // session's tabs are @1/@2, and hive-bump-deps is the other fixture).
      expect(byId.has('terminal:window:@1')).toBe(false)
      expect(byId.has('terminal:window:@2')).toBe(false)
      expect(byId.has('terminal:attach:hive-bump-deps')).toBe(false)

      wrapper.unmount()
    })

    it('launches a peer from the palette and hides the choices for a stopped session', async () => {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt('/terminal/hive-fix-parser')
      const palette = useCommandPalette()
      const results = paletteResults()
      palette.open.value = false
      await flushPromises()
      palette.open.value = true
      await flushPromises()
      palette.open.value = false
      palette.open.value = true
      await flushPromises()
      expect(mocks.SessionLaunchOptions).toHaveBeenCalledOnce()

      const command = results.value.find((candidate) => candidate.id === 'terminal:session:agent:codex')
      expect(command?.title).toBe('New codex agent')
      expect(command?.group).toBe('fix the parser')
      await command!.run()
      await flushPromises()
      expect(session.newAgentWindow).toHaveBeenCalledWith('codex')

      mocks.SessionStatuses.mockResolvedValue({
        items: [{ sessionId: '1', running: false, windows: [] }],
        pollIntervalMs: 60_000,
      })
      await useSessionStatuses().reload()
      await flushPromises()
      expect(results.value.some((candidate) => candidate.id.startsWith('terminal:session:agent:'))).toBe(false)
      palette.open.value = false
      wrapper.unmount()
    })

    it('runs a configured session action against the attached session', async () => {
      mocks.TerminalActionViews.mockImplementation(async (surface: string) =>
        surface === 'session' ? [{ id: 'standup', label: 'Post standup', type: 'shell', inputs: [] }] : [],
      )
      mocks.InvokeTerminalAction.mockResolvedValue(undefined)
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt('/terminal/hive-fix-parser')
      const results = paletteResults()

      const cmd = results.value.find((candidate) => candidate.id === 'terminal:session:terminal-action:standup')
      expect(cmd?.title).toBe('Post standup')
      expect(cmd?.group).toBe('fix the parser')

      await cmd!.run()
      await flushPromises()
      expect(mocks.InvokeTerminalAction).toHaveBeenCalledWith('standup', { slug: 'hive-fix-parser', windowId: '' }, {})

      wrapper.unmount()
    })

    // A window action needs a window, and the palette's is the one on screen.
    it('runs a configured window action against the active window', async () => {
      mocks.TerminalActionViews.mockImplementation(async (surface: string) =>
        surface === 'window' ? [{ id: 'tail-log', label: 'Tail log', type: 'shell', inputs: [] }] : [],
      )
      mocks.InvokeTerminalAction.mockResolvedValue(undefined)
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt('/terminal/hive-fix-parser')
      const results = paletteResults()

      const cmd = results.value.find((candidate) => candidate.id === 'terminal:window:action:terminal-action:tail-log')
      expect(cmd?.title).toBe('Tail log')
      expect(cmd?.group).toBe('fix the parser')
      // The label says what it does; the hint says what it does it to.
      expect(cmd?.hint).toBe('agent')

      await cmd!.run()
      await flushPromises()
      expect(mocks.InvokeTerminalAction).toHaveBeenCalledWith(
        'tail-log',
        { slug: 'hive-fix-parser', windowId: '@1' },
        {},
      )

      wrapper.unmount()
    })

    it('contributes nothing with no session attached', async () => {
      const { wrapper } = await mountAt()
      const results = paletteResults()

      // Attach rows live at App level, so an unattached mode has nothing
      // of its own to offer.
      expect(results.value.some((cmd) => cmd.id.startsWith('terminal:'))).toBe(false)

      wrapper.unmount()
    })

    it('withdraws its palette rows entirely off-screen', async () => {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const { wrapper } = await mountAt('/terminal/hive-fix-parser')
      const results = paletteResults()

      expect(results.value.some((cmd) => cmd.id.startsWith('terminal:session:'))).toBe(true)

      // Mounted but hidden behind another mode: the library must not follow.
      await wrapper.setProps({ active: false })
      expect(results.value.some((cmd) => cmd.id.startsWith('terminal:'))).toBe(false)

      wrapper.unmount()
    })
  })

  describe('session status bar', () => {
    async function mountWithStatusBar(session = fakeSession()) {
      useTerminalStatusBar().setShowStatusBar(true)
      const mounted = await mountAvailable(session)
      await mounted.wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()
      return mounted
    }

    it('stays out of the way until the setting turns it on', async () => {
      const { wrapper } = await mountAvailable()
      await wrapper.get('[data-testid="terminal-session-row"][data-slug="hive-fix-parser"]').trigger('click')
      await flushPromises()

      expect(wrapper.find('[data-testid="terminal-pane-statusbar"]').exists()).toBe(false)
      expect(mocks.SessionGitStatus).not.toHaveBeenCalled()

      wrapper.unmount()
    })

    it('reports the branch, diff and pull request without renaming the session', async () => {
      mocks.SessionGitStatus.mockResolvedValue({
        path: '/tmp/fix-parser',
        branch: 'feat/parser',
        dirty: true,
        unpushed: true,
        additions: 42,
        deletions: 7,
        host: 'github.com',
        owner: 'hay-kot',
        repo: 'hive',
        resolved: true,
        error: '',
      })
      mocks.SessionPullRequest.mockResolvedValue({
        status: 'found',
        number: 311,
        title: 'Fix the parser',
        state: 'OPEN',
        isDraft: false,
        url: 'https://github.com/hay-kot/hive/pull/311',
        reviewDecision: 'APPROVED',
        checks: 'passing',
      })

      const { wrapper } = await mountWithStatusBar()

      // The sidebar already names the session, so the bar does not.
      expect(wrapper.find('[data-testid="terminal-pane-statusbar-workspace"]').exists()).toBe(false)
      expect(wrapper.get('[data-testid="session-status-branch"]').text()).toBe('feat/parser')
      expect(wrapper.get('[data-testid="session-status-diff"]').text()).toBe('+42−7')
      // Icon-only, so the tooltip is the whole explanation and has to exist —
      // an aria-label renders none, which is what left the arrow a mystery.
      expect(tooltipFor(wrapper, 'session-status-dirty')).toBe('Uncommitted changes in this checkout')
      expect(tooltipFor(wrapper, 'session-status-unpushed')).toBe(
        'Commits on this branch that the remote does not have',
      )
      expect(wrapper.get('[data-testid="session-status-pr"]').text()).toContain('#311')
      expect(wrapper.get('[data-testid="session-status-checks"]').text()).toBe('passing')
      // The lookup is keyed by what git resolved, not by anything read twice.
      expect(mocks.SessionPullRequest).toHaveBeenCalledWith(
        { host: 'github.com', owner: 'hay-kot', repo: 'hive', branch: 'feat/parser' },
        false,
      )

      wrapper.unmount()
    })

    // Reported continuously, not only read on a click, so App.vue can scope
    // Tasks to the attached session's repo from any entry point.
    it('reports the attached session’s resolved owner/repo as it settles', async () => {
      mocks.SessionGitStatus.mockResolvedValue({
        path: '/tmp/fix-parser',
        branch: 'feat/parser',
        dirty: false,
        unpushed: false,
        additions: 0,
        deletions: 0,
        host: 'github.com',
        owner: 'hay-kot',
        repo: 'hive',
        resolved: true,
        error: '',
      })

      const { wrapper } = await mountWithStatusBar()

      expect(wrapper.emitted('session-repo-key')).toContainEqual(['hay-kot/hive'])

      wrapper.unmount()
    })

    // A remote naming no host resolves owner/repo empty, and a failed or
    // pending read never carries a stale guess forward.
    it('reports an empty repo key for a resolved hostless remote', async () => {
      mocks.SessionGitStatus.mockResolvedValue({
        path: '/tmp/fix-parser',
        branch: 'main',
        dirty: false,
        unpushed: false,
        additions: 0,
        deletions: 0,
        host: '',
        owner: '',
        repo: '',
        resolved: true,
        error: '',
      })

      const { wrapper } = await mountWithStatusBar()

      const emissions = wrapper.emitted('session-repo-key') ?? []
      expect(emissions.at(-1)).toEqual([''])

      wrapper.unmount()
    })

    it('opens the pull request in a browser when its chip is clicked', async () => {
      mocks.SessionPullRequest.mockResolvedValue({
        status: 'found',
        number: 311,
        title: 'Fix the parser',
        state: 'OPEN',
        isDraft: false,
        url: 'https://github.com/hay-kot/hive/pull/311',
        reviewDecision: '',
        checks: '',
      })

      const { wrapper } = await mountWithStatusBar()
      await wrapper.get('[data-testid="session-status-pr"]').trigger('click')

      expect(mocks.OpenURL).toHaveBeenCalledWith('https://github.com/hay-kot/hive/pull/311')

      wrapper.unmount()
    })

    it('copies the pull request as a Markdown link', async () => {
      mocks.SessionPullRequest.mockResolvedValue({
        status: 'found',
        number: 311,
        title: 'Fix the parser',
        state: 'OPEN',
        isDraft: false,
        url: 'https://github.com/hay-kot/hive/pull/311',
        reviewDecision: '',
        checks: '',
        additions: 420,
        deletions: 37,
      })

      const { wrapper } = await mountWithStatusBar()

      await wrapper.get('[data-testid="session-status-copy"]').trigger('click')
      await flushPromises()
      expect(mocks.SetClipboardText).toHaveBeenCalledWith(
        '[[hive] Fix the parser `(+420, -37)`](https://github.com/hay-kot/hive/pull/311)',
      )

      wrapper.unmount()
    })

    // The branch is the only thing in the row that can be re-read on hover, so
    // everything else must survive a narrow pane whole.
    it('lets only the branch give up width when the row overflows', async () => {
      mocks.SessionGitStatus.mockResolvedValue({
        path: '/tmp/fix-parser',
        branch: 'feat/parser',
        dirty: true,
        unpushed: true,
        additions: 420,
        deletions: 37,
        host: 'github.com',
        owner: 'hay-kot',
        repo: 'hive',
        resolved: true,
        error: '',
      })
      mocks.SessionPullRequest.mockResolvedValue({
        status: 'found',
        number: 311,
        title: 'Fix the parser',
        state: 'OPEN',
        isDraft: false,
        url: 'https://github.com/hay-kot/hive/pull/311',
        reviewDecision: '',
        checks: 'passing',
        additions: 420,
        deletions: 37,
      })

      const { wrapper } = await mountWithStatusBar()

      const branch = wrapper.get('[data-testid="session-status-branch"]')
      expect(branch.classes()).toContain('min-w-0')
      expect(branch.get('.font-mono').classes()).toContain('truncate')

      for (const testid of ['session-status-diff', 'session-status-pr', 'session-status-copy']) {
        expect(wrapper.get(`[data-testid="${testid}"]`).classes()).toContain('shrink-0')
      }

      // The branch carries no tooltip, so a name cut this way cannot be read
      // back. Pinned so that stays a decision rather than an oversight.
      expect(tooltipFor(wrapper, 'session-status-branch')).toBe('')

      wrapper.unmount()
    })

    // Animating a cached answer animates nothing arriving: it was known before
    // the bar painted. Only a read that actually went to the network eases in.
    it('eases the pull request in only when it just arrived', async () => {
      const found = {
        status: 'found',
        number: 311,
        title: 'Fix the parser',
        state: 'OPEN',
        isDraft: false,
        url: 'https://github.com/hay-kot/hive/pull/311',
        reviewDecision: '',
        checks: '',
        additions: 420,
        deletions: 37,
      }
      mocks.SessionPullRequest.mockResolvedValue({ ...found, cached: false })

      // Asserted on the computed rather than the rendered Transition because
      // this suite stubs transitions out, so the prop never reaches the DOM.
      const fresh = await mountWithStatusBar()
      expect(fresh.wrapper.findComponent({ name: 'SessionStatusChips' }).vm.animateArrival).toBe(true)
      fresh.wrapper.unmount()

      mocks.SessionPullRequest.mockResolvedValue({ ...found, cached: true })

      const cachedMount = await mountWithStatusBar()
      expect(cachedMount.wrapper.findComponent({ name: 'SessionStatusChips' }).vm.animateArrival).toBe(false)
      cachedMount.wrapper.unmount()
    })

    it('offers no copy button for a branch with no pull request to share', async () => {
      const { wrapper } = await mountWithStatusBar()

      expect(wrapper.find('[data-testid="session-status-copy"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('still offers the buttons for a session with no pull request', async () => {
      const { wrapper } = await mountWithStatusBar()

      expect(wrapper.find('[data-testid="session-status-pr"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="session-status-pr-error"]').exists()).toBe(false)

      await wrapper.get('[data-testid="terminal-pane-statusbar-open-editor"]').trigger('click')
      await wrapper.get('[data-testid="terminal-pane-statusbar-reveal"]').trigger('click')
      await flushPromises()
      expect(mocks.OpenSessionInEditor).toHaveBeenCalledWith('1')
      expect(mocks.RevealSession).toHaveBeenCalledWith('1')

      wrapper.unmount()
    })

    // The button only signals the click; App.vue reads the repo to scope to
    // off the session-repo-key it has already been keeping current.
    it('emits open-tasks, carrying no payload of its own, when the tasks button is clicked', async () => {
      const { wrapper } = await mountWithStatusBar()

      await wrapper.get('[data-testid="terminal-statusbar-tasks"]').trigger('click')

      expect(wrapper.emitted('open-tasks')).toEqual([[]])

      wrapper.unmount()
    })

    // The bar must never say "no pull request" because the lookup broke: the
    // branch may well have one, and the user would act on the wrong fact.
    it('reports a failed pull-request lookup as a failure, not as having none', async () => {
      mocks.SessionPullRequest.mockRejectedValue(new Error('Bad credentials'))

      const { wrapper } = await mountWithStatusBar()

      expect(wrapper.find('[data-testid="session-status-pr"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="session-status-pr-error"]').exists()).toBe(true)
      expect(tooltipFor(wrapper, 'session-status-pr-error')).toContain('Bad credentials')

      wrapper.unmount()
    })

    it('surfaces a failed git read instead of showing a clean branch it never saw', async () => {
      mocks.SessionGitStatus.mockResolvedValue({
        path: '/tmp/fix-parser',
        branch: 'feat/parser',
        dirty: false,
        unpushed: false,
        additions: 0,
        deletions: 0,
        host: '',
        owner: '',
        repo: '',
        resolved: true,
        error: 'git status: exit 128',
      })

      const { wrapper } = await mountWithStatusBar()

      expect(wrapper.find('[data-testid="session-status-dirty"]').exists()).toBe(false)
      expect(tooltipFor(wrapper, 'session-status-git-error')).toBe('git status: exit 128')

      wrapper.unmount()
    })

    it('gives the scratch terminal no bar, because it has no checkout', async () => {
      useTerminalStatusBar().setShowStatusBar(true)
      const { wrapper } = await mountAvailable()
      await wrapper.get('[data-testid="terminal-scratch-heading"]').trigger('click')
      await flushPromises()

      expect(wrapper.find('[data-testid="terminal-pane-statusbar"]').exists()).toBe(false)
      expect(mocks.SessionGitStatus).not.toHaveBeenCalled()

      wrapper.unmount()
    })

    it('surfaces an open that failed and clears it on the next attempt', async () => {
      mocks.OpenSessionInEditor.mockRejectedValueOnce(new Error('editor "zed" was not found on PATH'))

      const { wrapper } = await mountWithStatusBar()

      await wrapper.get('[data-testid="terminal-pane-statusbar-open-editor"]').trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-testid="terminal-pane-statusbar-error"]').text()).toBe(
        'editor "zed" was not found on PATH',
      )

      await wrapper.get('[data-testid="terminal-pane-statusbar-open-editor"]').trigger('click')
      await flushPromises()
      expect(wrapper.find('[data-testid="terminal-pane-statusbar-error"]').exists()).toBe(false)

      wrapper.unmount()
    })
  })

  // App.vue resolves whose canvases the attached session reads and hands it
  // down; the mode's part is the pane beside the terminal and the route query
  // that opens it.
  describe('canvas pane', () => {
    const canvas = { workspace: 'hay-kot/hive', session: '1' }

    async function mountWithCanvas(path: string) {
      const session = fakeSession()
      mocks.useTerminalWindows.mockReturnValue(session)
      const router = createAppRouter(createMemoryHistory())
      await router.push(path)
      await router.isReady()
      const wrapper = mount(TerminalMode, {
        props: { canvas },
        global: { plugins: [router], stubs: { transition: true, 'transition-group': true } },
      })
      await flushPromises()
      return { wrapper, router, session }
    }

    it('stays closed until the route asks for it', async () => {
      const { wrapper } = await mountWithCanvas('/terminal/hive-fix-parser')

      expect(wrapper.find('[data-testid="agent-canvas-pane"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('opens beside the session and closes back to the terminal alone', async () => {
      const { wrapper, router } = await mountWithCanvas('/terminal/hive-fix-parser?canvas=1')

      expect(wrapper.find('[data-testid="agent-canvas-pane"]').exists()).toBe(true)

      await wrapper.get('[data-testid="agent-canvas-close"]').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.query.canvas).toBeUndefined()
      expect(router.currentRoute.value.params.slug).toBe('hive-fix-parser')
      expect(wrapper.find('[data-testid="agent-canvas-pane"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('keeps the pane mounted while the active terminal window changes', async () => {
      const { wrapper, router, session } = await mountWithCanvas(
        '/terminal/hive-fix-parser?canvas=mermaid-architecture',
      )
      const pane = wrapper.get('[data-testid="agent-canvas-pane"]').element
      const visited: Array<Record<string, unknown>> = []
      router.afterEach((to) => visited.push({ ...to.query }))

      session.activeWindowId.value = '@2'
      await flushPromises()

      expect(visited).toEqual([{ window: '@2', canvas: 'mermaid-architecture' }])
      expect(wrapper.get('[data-testid="agent-canvas-pane"]').element).toBe(pane)

      wrapper.unmount()
    })

    // No agent client in this spec, so the pane reads no canvases: the state
    // a session is in before its agent has the server.
    it('leads from the empty pane to the MCP settings page', async () => {
      const { wrapper, router } = await mountWithCanvas('/terminal/hive-fix-parser?canvas=1')

      await wrapper.get('[data-testid="agent-canvas-setup-open"]').trigger('click')
      await flushPromises()

      expect(router.currentRoute.value.name).toBe('application-settings')
      expect(router.currentRoute.value.params.section).toBe('mcp')

      wrapper.unmount()
    })

    it("asks for the full-page view on the session's repository", async () => {
      const { wrapper } = await mountWithCanvas('/terminal/hive-fix-parser?canvas=1')

      await wrapper.get('[data-testid="agent-canvas-expand"]').trigger('click')

      expect(wrapper.emitted('open-canvas-page')).toEqual([[{ workspace: 'hay-kot/hive', session: '1', name: null }]])

      wrapper.unmount()
    })
  })
})
