import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import App from '../App.vue'
import { useCommandPalette } from '../composables/useCommands'
import { resetNewSessionForTests } from '../composables/useNewSession'
import { resetToastsForTests } from '../composables/useToasts'
import { requestedEditorFilter } from '../keybindings/keymapRows'
import { useReportDialog } from '../composables/useReportDialog'
import { useActivity } from '../stores/useActivity'
import { resetFlowsSessionForTests, useFlowsSession } from '../pipeline/composables/useFlowsSession'
import { resetNotificationSettingsForTests } from '../composables/useNotificationSettings'
import { usePopupTerminal } from '../stores/usePopupTerminal'
import { resetLaunchersForTests } from '../composables/useLaunchers'
import { formatCombo, SEQUENCE_TIMEOUT_MS, useKeybindings } from '../composables/useKeybindings'
import { defaultTerminalFontSizePx, useTerminalFont } from '../stores/useTerminalFont'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import {
  resetAttachedTerminalWindowsForTests,
  setAttachedTerminalWindows,
} from '../composables/useAttachedTerminalWindows'
import { useAgentSessionsAll } from '../stores/useAgentSessionsAll'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'
import type { AgentWorkspacesClient } from '../lib/agentWorkspacesClient'
import { useTasks } from '../stores/useTasks'
import { applicationSettingsSections, createAppRouter } from '../router'
import TerminalMode from '../components/TerminalMode.vue'
import { setAgentsTreeHandles } from '../lib/agentsTree'
import { setTerminalTreeHandles, type TerminalTreeHandles } from '../lib/terminalTree'
import { ListSessions } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'

const mocks = vi.hoisted(() => ({
  // flowsservice
  ListFlows: vi.fn(),
  GetFlow: vi.fn(),
  CreateFlow: vi.fn(),
  SeedStarterFlow: vi.fn(),
  RenameFlow: vi.fn(),
  SetFlowEnabled: vi.fn(),
  DeleteFlow: vi.fn(),
  GetLayout: vi.fn(),
  SaveFlow: vi.fn(),
  SaveLayout: vi.fn(),
  GetSidebar: vi.fn(),
  SaveSidebar: vi.fn(),
  // actionsservice
  ListActions: vi.fn(),
  CreateAction: vi.fn(),
  UpdateAction: vi.fn(),
  DeleteAction: vi.fn(),
  // pipelineservice
  ListByFeed: vi.fn(),
  ListArchivedByFeed: vi.fn(),
  ListTrash: vi.fn(),
  FindItems: vi.fn(),
  Feed: vi.fn(),
  FeedCounts: vi.fn(),
  SetUnread: vi.fn(),
  ToggleArchived: vi.fn(),
  ToggleIgnored: vi.fn(),
  Events: vi.fn(),
  ActionRun: vi.fn(),
  SessionLaunchOptions: vi.fn(),
  CreateSession: vi.fn(),
  FailedSessionDraft: vi.fn(),
  DismissFailedSession: vi.fn(),
  NewSessionDraft: vi.fn(),
  ActionViews: vi.fn(),
  InvokeAction: vi.fn(),
  NodeRuns: vi.fn(),
  // github connection service
  Status: vi.fn(),
  StartDeviceFlow: vi.fn(),
  CancelDeviceFlow: vi.fn(),
  SetToken: vi.fn(),
  Disconnect: vi.fn(),
  // updaterservice
  UpdaterStatus: vi.fn(),
  InstallUpdate: vi.fn(),
  // notification settings
  NotificationSettings: vi.fn(),
  SetNotificationSettings: vi.fn(),
  PermissionStatus: vi.fn(),
  RequestNotificationPermission: vi.fn(),
  Notify: vi.fn(),
  // useTasks() calls useWindowFocus() at module scope, so Focused() runs the
  // instant App.vue's import of TasksView pulls that module in — before any
  // beforeEach can set it up. Resolve it here rather than there.
  Focused: vi.fn().mockResolvedValue(true),
  ActivityList: vi.fn(),
  RecordActivity: vi.fn(),
  // tasksservice
  DeleteTask: vi.fn(),
  ListTasks: vi.fn(),
  PruneTasks: vi.fn(),
  SetTaskStatus: vi.fn(),
  TaskDetail: vi.fn(),
  TaskRepoKeys: vi.fn(),
  // terminalservice
  TerminalAvailable: vi.fn(),
  TerminalEndpoint: vi.fn(),
  // popupterminalservice
  PopupAvailable: vi.fn(),
  PopupEndpoint: vi.fn(),
  PopupLaunchers: vi.fn(),
  // agentsservice
  AgentsAvailable: vi.fn(),
  AgentsEndpoint: vi.fn(),
  // A usable Hive setup keeps first run off in unrelated tests, like the
  // connected GitHub default elsewhere in this file.
  HiveSetup: vi.fn(),
  SaveHiveSetup: vi.fn(),
  InspectWorkspace: vi.fn(),
  ChooseDirectory: vi.fn(),
  // First run defaults to completed so unrelated tests bypass it.
  OnboardingSettings: vi.fn(),
  SetOnboardingCompleted: vi.fn(),
  // runtime
  On: vi.fn().mockReturnValue(() => {}),
  Hide: vi.fn(),
  // A test installs a fake Agents client here; null keeps the real one.
  agentsClient: null as AgentWorkspacesClient | null,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/flowsservice', () => ({
  ListFlows: mocks.ListFlows,
  GetFlow: mocks.GetFlow,
  CreateFlow: mocks.CreateFlow,
  SeedStarterFlow: mocks.SeedStarterFlow,
  RenameFlow: mocks.RenameFlow,
  SetFlowEnabled: mocks.SetFlowEnabled,
  DeleteFlow: mocks.DeleteFlow,
  GetLayout: mocks.GetLayout,
  SaveFlow: mocks.SaveFlow,
  SaveLayout: mocks.SaveLayout,
  GetSidebar: mocks.GetSidebar,
  SaveSidebar: mocks.SaveSidebar,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/actionsservice', () => ({
  ListActions: mocks.ListActions,
  CreateAction: mocks.CreateAction,
  UpdateAction: mocks.UpdateAction,
  DeleteAction: mocks.DeleteAction,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice', () => ({
  ListByFeed: mocks.ListByFeed,
  ListArchivedByFeed: mocks.ListArchivedByFeed,
  ListTrash: mocks.ListTrash,
  FindItems: mocks.FindItems,
  Feed: mocks.Feed,
  FeedCounts: mocks.FeedCounts,
  SetUnread: mocks.SetUnread,
  ToggleArchived: mocks.ToggleArchived,
  ToggleIgnored: mocks.ToggleIgnored,
  Events: mocks.Events,
  ActionRun: mocks.ActionRun,
  NewSessionDraft: mocks.NewSessionDraft,
  ActionViews: mocks.ActionViews,
  InvokeAction: mocks.InvokeAction,
  NodeRuns: mocks.NodeRuns,
}))
vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionLaunchOptions: mocks.SessionLaunchOptions,
  CreateSession: mocks.CreateSession,
  FailedSessionDraft: mocks.FailedSessionDraft,
  DismissFailedSession: mocks.DismissFailedSession,
  ListSessions: vi.fn().mockResolvedValue([]),
  SessionStatuses: vi.fn().mockResolvedValue({ items: [], pollIntervalMs: 60_000 }),
  TerminalActionViews: vi.fn().mockResolvedValue([]),
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/githubservice', () => ({
  Status: mocks.Status,
  StartDeviceFlow: mocks.StartDeviceFlow,
  CancelDeviceFlow: mocks.CancelDeviceFlow,
  SetToken: mocks.SetToken,
  Disconnect: mocks.Disconnect,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/updaterservice', () => ({
  Status: mocks.UpdaterStatus,
  InstallUpdate: mocks.InstallUpdate,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  NotificationSettings: mocks.NotificationSettings,
  SetNotificationSettings: mocks.SetNotificationSettings,
  OnboardingSettings: mocks.OnboardingSettings,
  SetOnboardingCompleted: mocks.SetOnboardingCompleted,
  AppearanceSettings: vi.fn().mockResolvedValue({
    theme: '',
    terminalFontSizePx: 13,
    terminalFontFamily: '',
    terminalFontWeight: 0,
    terminalFontWeightBold: 0,
    terminalShowWindows: true,
    terminalPoolSize: 3,
  }),
  Fonts: vi.fn().mockResolvedValue({ all: [], monospace: [] }),
  SetTheme: vi.fn(),
  SetTerminalFontSize: vi.fn(),
  SetTerminalFontFamily: vi.fn(),
  SetTerminalFontWeights: vi.fn(),
  SetTerminalShowWindows: vi.fn(),
  SetTerminalPoolSize: vi.fn(),
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/notificationservice', () => ({
  PermissionStatus: mocks.PermissionStatus,
  RequestNotificationPermission: mocks.RequestNotificationPermission,
  Notify: mocks.Notify,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: mocks.Focused,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hiveconfigservice', () => ({
  Setup: mocks.HiveSetup,
  Save: mocks.SaveHiveSetup,
  InspectWorkspace: mocks.InspectWorkspace,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  ChooseDirectory: mocks.ChooseDirectory,
}))
// The Observability page owns frame sampling. Stub its rAF loop because App
// tests exercise settings routes without measuring their render cost.
vi.mock('../composables/useFrameStats', async () => {
  const { shallowRef } = await vi.importActual<typeof import('vue')>('vue')
  const stats = shallowRef({
    fps: 0,
    frameMs: 0,
    worstFrameMs: 0,
    dropped: 0,
    windowMs: 10_000,
    lagMs: 0,
    worstLagMs: 0,
    buckets: [],
  })
  return {
    startFrameStats: vi.fn(),
    stopFrameStats: vi.fn(),
    useFrameStats: () => ({ stats, running: shallowRef(false) }),
  }
})
vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/activityservice', () => ({
  List: mocks.ActivityList,
  Record: mocks.RecordActivity,
}))
vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/tasksservice', () => ({
  DeleteTask: mocks.DeleteTask,
  ListTasks: mocks.ListTasks,
  PruneTasks: mocks.PruneTasks,
  SetTaskStatus: mocks.SetTaskStatus,
  TaskDetail: mocks.TaskDetail,
  TaskRepoKeys: mocks.TaskRepoKeys,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Window: { Hide: mocks.Hide },
  Call: { ByID: vi.fn() },
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice', () => ({
  Available: mocks.TerminalAvailable,
  Endpoint: mocks.TerminalEndpoint,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice', () => ({
  Available: mocks.PopupAvailable,
  Endpoint: mocks.PopupEndpoint,
  Launchers: mocks.PopupLaunchers,
}))

vi.mock('../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.AgentsAvailable,
  Endpoint: mocks.AgentsEndpoint,
}))
vi.mock('../lib/agentWorkspacesClient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/agentWorkspacesClient')>()
  return {
    ...actual,
    createAgentWorkspacesClient: (endpoint: Parameters<typeof actual.createAgentWorkspacesClient>[0]) =>
      mocks.agentsClient ?? actual.createAgentWorkspacesClient(endpoint),
  }
})

const flow = {
  id: 'personal',
  name: 'Personal',
  enabled: true,
  nodes: [
    { id: 'src', type: 'sources.github' },
    { id: 'desktop', type: 'feed', name: 'Desktop UI' },
  ],
  wires: [{ from: 'src', to: 'desktop' }],
}

function inboxItems() {
  return [1, 2].map((n) => ({
    id: n,
    profileId: 'personal',
    sourceKind: 'github',
    sourceScope: '',
    externalId: `pr-${n}`,
    title: n === 1 ? 'First' : 'Second',
    url: '',
    payload: { kind: 'PR', repo: 'acme/app', num: n, author: 'hay' },
    revision: 1,
    unread: false,
    lifecycle: 'active',
    firstSeenAt: 1,
    lastEventAt: 2,
  }))
}

async function mountAppWithRouter() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/')
  await router.isReady()
  const wrapper = mount(App, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

async function mountApp() {
  return (await mountAppWithRouter()).wrapper
}

// Terminal mode is mounted once and hidden on a trip to the hub, so whether it
// is the surface on screen is a question about visibility, never about the
// element being there. Read off v-show's own inline display rather than through
// isVisible(): that goes to getComputedStyle, which happy-dom does not resolve
// for a tree VTU never attached to the document.
function terminalOnScreen(wrapper: VueWrapper): boolean {
  const mode = wrapper.find('[data-testid="terminal-mode"]')
  return mode.exists() && !(mode.attributes('style') ?? '').includes('display: none')
}

// Same shape as terminalOnScreen: the Agents area is mount-once/v-show too.
function agentsOnScreen(wrapper: VueWrapper): boolean {
  const mode = wrapper.find('[data-testid="agents-mode"]')
  return mode.exists() && !(mode.attributes('style') ?? '').includes('display: none')
}

// Stands in for TerminalMode's registration. Every handle is a spy so a test
// only has to name the one it asserts on, and a command that reached the wrong
// handle still shows up.
function stubTerminalTree(overrides: Partial<TerminalTreeHandles> = {}): TerminalTreeHandles {
  const handles: TerminalTreeHandles = {
    focusTree: vi.fn(),
    focusPane: vi.fn(),
    focusFilter: vi.fn(),
    selectWindow: vi.fn(),
    newWindow: vi.fn(),
    closeWindow: vi.fn(),
    stepWindow: vi.fn(),
    splitPane: vi.fn(),
    closePane: vi.fn(),
    zoomPane: vi.fn(),
    focusPaneDirection: vi.fn(),
    ...overrides,
  }
  setTerminalTreeHandles(handles)
  return handles
}

// A pane in the document, so a keydown dispatched on it reads as one a focused
// terminal would have taken.
function focusedPane(): HTMLElement {
  const pane = document.createElement('div')
  pane.setAttribute('data-terminal-input-scope', '')
  document.body.append(pane)
  return pane
}

describe('App', () => {
  function usableHiveSetup() {
    return {
      config: {
        path: '/home/dev/.config/hive/config.yaml',
        exists: true,
        usable: true,
        unreadable: '',
        defaultAgent: 'claude',
        profiles: [{ name: 'claude', command: 'claude', flags: [] }],
        workspaces: [{ path: '/home/dev/code', exists: true, repos: 4 }],
      },
      agents: [
        {
          name: 'claude',
          label: 'Claude Code',
          skipPermissionFlags: ['--dangerously-skip-permissions'],
          installed: true,
        },
        {
          name: 'opencode',
          label: 'OpenCode',
          skipPermissionFlags: ['--agent', 'free-permissions-runner'],
          installed: false,
        },
      ],
      defaultAgentOverride: '',
    }
  }

  function emptyHiveSetup() {
    const base = usableHiveSetup()
    return {
      ...base,
      config: { ...base.config, exists: false, usable: false, defaultAgent: '', profiles: [], workspaces: [] },
    }
  }

  beforeEach(() => {
    // useFlowsSession is a module singleton (App.vue + FlowsView.vue share
    // one instance) — without this, a later test would silently reuse a
    // prior test's instance, including its already-torn-down onMounted/
    // watch hooks from that test's wrapper.unmount().
    resetFlowsSessionForTests()
    // useNotificationSettings is a module singleton too — reset it so a test's
    // resolved permission state cannot leak into the next test's first-run walk.
    resetNotificationSettingsForTests()
    // The launcher commands are a module singleton too, and a launcher
    // registered by one test would stay bindable in the next.
    resetLaunchersForTests()
    useKeybindings().clearAll()
    useKeybindings().clearPendingSequence()
    requestedEditorFilter.value = null
    resetAttachedTerminalWindowsForTests()
    vi.clearAllMocks()
    // Panel collapse / width state persists via useStorage; clear it so one
    // test's collapsed sidebar can't leak into the next.
    localStorage.clear()
    mocks.Status.mockResolvedValue({
      state: 'connected',
      login: 'octocat',
      name: 'Octocat',
      avatarUrl: '',
      message: '',
    })
    mocks.ListFlows.mockResolvedValue([{ id: 'personal', name: 'Personal', enabled: true, valid: true }])
    mocks.GetFlow.mockResolvedValue(flow)
    mocks.GetLayout.mockResolvedValue({ nodes: {} })
    mocks.GetSidebar.mockResolvedValue({ items: [] })
    mocks.SaveSidebar.mockResolvedValue(undefined)
    mocks.ListByFeed.mockResolvedValue([])
    mocks.ListArchivedByFeed.mockResolvedValue([])
    mocks.ListTrash.mockResolvedValue([])
    mocks.FindItems.mockResolvedValue([])
    mocks.Feed.mockResolvedValue('')
    mocks.FeedCounts.mockResolvedValue([{ feedId: 'personal/desktop', total: 1, unread: 0, archived: 0 }])
    mocks.Events.mockResolvedValue([])
    mocks.ActionRun.mockResolvedValue({ commandId: 1, status: 'done' })
    mocks.SessionLaunchOptions.mockResolvedValue({
      repositories: [],
      defaultRepository: '',
      agents: [],
      defaultAgent: '',
    })
    mocks.ActionViews.mockResolvedValue([])
    mocks.InvokeAction.mockResolvedValue(undefined)
    mocks.ListActions.mockResolvedValue({ actions: [], error: '' })
    mocks.NodeRuns.mockResolvedValue([])
    mocks.RenameFlow.mockResolvedValue({ id: 'personal', name: 'Team', enabled: true, valid: true })
    mocks.SetFlowEnabled.mockImplementation(async (id: string, enabled: boolean) => ({
      id,
      name: 'Personal',
      enabled,
      valid: true,
    }))
    mocks.DeleteFlow.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
    mocks.FailedSessionDraft.mockResolvedValue({ repository: '', name: '', prompt: '' })
    mocks.DismissFailedSession.mockResolvedValue(undefined)
    mocks.UpdaterStatus.mockResolvedValue({
      enabled: true,
      available: false,
      currentVersion: 'dev',
      latestVersion: '',
      notes: '',
    })
    mocks.InstallUpdate.mockResolvedValue(undefined)
    mocks.NotificationSettings.mockResolvedValue({
      notificationsEnabled: true,
      systemNotificationsEnabled: true,
      notificationSound: true,
    })
    mocks.SetNotificationSettings.mockResolvedValue(undefined)
    mocks.PermissionStatus.mockResolvedValue('not-requested')
    mocks.RequestNotificationPermission.mockResolvedValue(true)
    mocks.Notify.mockResolvedValue(undefined)
    mocks.Focused.mockResolvedValue(true)
    mocks.ActivityList.mockResolvedValue([])
    mocks.RecordActivity.mockResolvedValue(undefined)
    mocks.ListTasks.mockResolvedValue([])
    mocks.TaskRepoKeys.mockResolvedValue([])
    mocks.PopupAvailable.mockResolvedValue({ available: true, reason: '' })
    mocks.PopupEndpoint.mockResolvedValue({ httpBaseURL: '', wsURL: '', token: '' })
    mocks.PopupLaunchers.mockResolvedValue([])
    mocks.TerminalAvailable.mockResolvedValue({ available: false, reason: 'tmux is not installed.' })
    mocks.TerminalEndpoint.mockResolvedValue({
      httpBaseURL: 'http://127.0.0.1:1',
      wsURL: 'ws://127.0.0.1:1/s',
      token: 'test',
    })
    mocks.agentsClient = null
    mocks.AgentsAvailable.mockResolvedValue({ available: false, reason: 'no ptyterm on this build.' })
    mocks.AgentsEndpoint.mockResolvedValue({
      httpBaseURL: 'http://127.0.0.1:1',
      wsURL: 'ws://127.0.0.1:1/s',
      token: 'test',
    })
    mocks.HiveSetup.mockResolvedValue(usableHiveSetup())
    mocks.SaveHiveSetup.mockImplementation(async () => usableHiveSetup())
    mocks.InspectWorkspace.mockResolvedValue({ path: '/home/dev/code', exists: true, repos: 4 })
    mocks.ChooseDirectory.mockResolvedValue('/home/dev/code')
    mocks.OnboardingSettings.mockResolvedValue({ completed: true })
    mocks.SetOnboardingCompleted.mockResolvedValue(undefined)
  })

  function firstRun() {
    mocks.OnboardingSettings.mockResolvedValue({ completed: false })
    mocks.Status.mockResolvedValue({ state: 'disconnected', login: '', name: '', avatarUrl: '', message: '' })
    mocks.ListFlows.mockResolvedValue([{ id: 'default', name: 'Default', enabled: true, valid: true, nodes: 0 }])
    mocks.SeedStarterFlow.mockResolvedValue({ id: 'default', name: 'Default', enabled: true, valid: true, nodes: 9 })
  }

  // Device-flow grants arrive through connection:updated, not a call result.
  async function connectGitHub() {
    mocks.Status.mockResolvedValue({
      state: 'connected',
      login: 'octocat',
      name: 'Octocat',
      avatarUrl: '',
      message: '',
    })
    const connection = mocks.On.mock.calls.find(([event]) => event === 'connection:updated')?.[1] as
      ((ev: { data: string }) => void) | undefined
    expect(connection).toBeDefined()
    connection?.({ data: 'github' })
    await flushPromises()
  }

  // A test that arms the deferred-sequence timer switches to fake timers; this
  // guarantees the next test always starts on real ones, even if an assertion
  // above throws before a test's own vi.useRealTimers() runs.
  // The toast stack and the New Session form are app-lifetime module state, so
  // without this a test hands the next one an overlay that swallows its keys.
  afterEach(() => {
    vi.useRealTimers()
    Reflect.deleteProperty(navigator, 'userAgent')
    resetToastsForTests()
    resetNewSessionForTests()
  })

  // ── First run ──────────────────────────────────────────────────────────────
  // GitHub does not gate the app, and a Default profile already exists, so
  // only the persisted marker gates first run.

  // The Hive step is first because it is the one answer the rest of the app
  // reads back: with no agent and no repository folder, the new session picker
  // has nothing in it.
  it('walks first run from the hive step into the connect step', async () => {
    firstRun()
    mocks.HiveSetup.mockResolvedValue(emptyHiveSetup())
    mocks.ChooseDirectory.mockResolvedValue('/home/dev/code')
    mocks.InspectWorkspace.mockResolvedValue({ path: '/home/dev/code', exists: true, repos: 6 })
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="hive-setup-agents"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(false)

    expect(wrapper.get('[data-testid="hive-agent-claude"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('[data-testid="hive-add-workspace"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="onboarding-hive-submit"]').trigger('click')
    await flushPromises()

    expect(mocks.SaveHiveSetup).toHaveBeenCalledWith(
      expect.objectContaining({
        defaultAgent: 'claude',
        workspaces: ['/home/dev/code'],
      }),
    )
    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('skips the hive step onto the connect step, without writing anything', async () => {
    firstRun()
    mocks.HiveSetup.mockResolvedValue(emptyHiveSetup())
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="onboarding-hive-skip"]').trigger('click')
    await flushPromises()

    expect(mocks.SaveHiveSetup).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('confirms an existing hive config instead of asking for one', async () => {
    firstRun()
    const wrapper = await mountApp()

    expect(wrapper.get('[data-testid="onboarding-hive-existing"]').text()).toContain('claude')
    expect(wrapper.find('[data-testid="hive-setup-agents"]').exists()).toBe(false)

    await wrapper.get('[data-testid="onboarding-hive-continue"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(true)

    wrapper.unmount()
  })

  // A returning user is never taken over by the walk, even with a config this
  // build considers unusable: Settings ▸ Hive CLI is where that is repaired.
  it('leaves a returning user alone when the hive config is unusable', async () => {
    mocks.HiveSetup.mockResolvedValue(emptyHiveSetup())
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)

    wrapper.unmount()
  })

  // An install that predates the marker walks first run once, and every step
  // with its own signal skips itself: a usable config is confirmed, a
  // connected account and a resolved grant are not asked about again.
  it('takes a configured install straight from the config confirmation to the hand-off', async () => {
    mocks.OnboardingSettings.mockResolvedValue({ completed: false })
    mocks.PermissionStatus.mockResolvedValue('granted')
    const wrapper = await mountApp()

    await passHiveStep(wrapper)

    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="onboarding-permissions-allow"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="onboarding-agent-start"]').exists()).toBe(true)
    expect(mocks.SeedStarterFlow).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  // A read that fails must not hold the whole app behind a step it cannot
  // populate.
  it('opens the app when the hive config cannot be read at all', async () => {
    mocks.HiveSetup.mockRejectedValue(new Error('no Hive config path is available'))
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Personal')

    wrapper.unmount()
  })

  // A file that exists but does not parse is not this app's to rewrite — the
  // backend refuses every save of it — so the step would be a form that can
  // only fail. First run goes straight to connecting; Settings ▸ Hive CLI
  // shows the parse error.
  it('skips the hive step when the config exists but does not parse', async () => {
    firstRun()
    const base = emptyHiveSetup()
    mocks.HiveSetup.mockResolvedValue({
      ...base,
      config: { ...base.config, exists: true, unreadable: 'yaml: line 3: did not find expected key' },
    })
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="hive-setup-agents"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="onboarding-hive-existing"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('keeps first run on the hive step when the save fails', async () => {
    firstRun()
    mocks.HiveSetup.mockResolvedValue(emptyHiveSetup())
    mocks.ChooseDirectory.mockResolvedValue('/home/dev/code')
    mocks.InspectWorkspace.mockResolvedValue({ path: '/home/dev/code', exists: true, repos: 6 })
    mocks.SaveHiveSetup.mockRejectedValue(new Error('saving the Hive configuration: permission denied'))
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="hive-add-workspace"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="onboarding-hive-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="onboarding-hive-error"]').text()).toContain('permission denied')
    expect(wrapper.find('[data-testid="hive-setup-agents"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('opens the feed with GitHub disconnected — the app is not gated on it', async () => {
    mocks.Status.mockResolvedValue({ state: 'disconnected', login: '', name: '', avatarUrl: '', message: '' })
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Personal')

    wrapper.unmount()
  })

  async function passHiveStep(wrapper: VueWrapper) {
    await wrapper.get('[data-testid="onboarding-hive-continue"]').trigger('click')
    await flushPromises()
  }

  it('walks first run: connect seeds the default profile, then the grant, then the hand-off', async () => {
    firstRun()
    const wrapper = await mountApp()
    await passHiveStep(wrapper)

    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="onboarding-connect"]').isVisible()).toBe(true)
    expect(mocks.CreateFlow).not.toHaveBeenCalled()
    expect(mocks.SeedStarterFlow).not.toHaveBeenCalled()

    await connectGitHub()
    expect(mocks.SeedStarterFlow).toHaveBeenCalledWith('default')

    expect(wrapper.get('[data-testid="onboarding-permissions-allow"]').isVisible()).toBe(true)
    await wrapper.get('[data-testid="onboarding-permissions-skip"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="onboarding-agent-start"]').isVisible()).toBe(true)
    expect(mocks.SetOnboardingCompleted).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="onboarding-agent-skip"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)
    expect(mocks.SetOnboardingCompleted).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Default')

    wrapper.unmount()
  })

  it('does not seed a profile that already has nodes', async () => {
    firstRun()
    mocks.ListFlows.mockResolvedValue([{ id: 'default', name: 'Default', enabled: true, valid: true, nodes: 9 }])
    const wrapper = await mountApp()
    await passHiveStep(wrapper)

    await connectGitHub()

    expect(mocks.SeedStarterFlow).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="onboarding-permissions-allow"]').isVisible()).toBe(true)

    wrapper.unmount()
  })

  it('asks for notification permission after connecting, then grants and reaches the hand-off', async () => {
    firstRun()
    const wrapper = await mountApp()
    await passHiveStep(wrapper)
    await connectGitHub()

    // The permission prompt is asked here, deliberately — not lazily mid-usage.
    expect(wrapper.get('[data-testid="onboarding-permissions-allow"]').isVisible()).toBe(true)
    mocks.PermissionStatus.mockResolvedValue('granted')
    await wrapper.get('[data-testid="onboarding-permissions-allow"]').trigger('click')
    await flushPromises()
    expect(mocks.RequestNotificationPermission).toHaveBeenCalledOnce()

    await wrapper.get('[data-testid="onboarding-permissions-finish"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="onboarding-agent-start"]').exists()).toBe(true)

    wrapper.unmount()
  })

  // The hand-off opens the seeded Hive workspace on the interview chat. First
  // run ends before the route changes, so the Agents area is not gated behind
  // the screen handing off to it.
  it('hands off into the Hive workspace chat and ends first run', async () => {
    firstRun()
    mocks.AgentsAvailable.mockResolvedValue({ available: true, reason: '' })
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/terminal/agents/sessions/first-run')) {
        return new Response(
          JSON.stringify({
            id: 7,
            workspace: 'hive',
            name: 'Getting started',
            agent: 'claude',
            lastOpenedAt: 0,
            slug: 'agentws-7',
            terminalId: '',
            windowId: '',
            paneId: '',
            cols: 0,
            rows: 0,
            resumeAttempted: true,
            notice: '',
            scheduleId: '',
          }),
          { status: 200 },
        )
      }
      return new Response('{}', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)
    const { wrapper, router } = await mountAppWithRouter()
    const push = vi.spyOn(router, 'push').mockResolvedValue(undefined)
    await passHiveStep(wrapper)
    await connectGitHub()
    await wrapper.get('[data-testid="onboarding-permissions-skip"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="onboarding-agent-start"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith(
      'http://127.0.0.1:1/api/terminal/agents/sessions/first-run',
      expect.objectContaining({ method: 'POST' }),
    )
    expect(mocks.SetOnboardingCompleted).toHaveBeenCalledOnce()
    expect(push).toHaveBeenCalledWith({ name: 'agents', params: { workspace: 'hive' }, query: { chat: '7' } })
    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)

    vi.unstubAllGlobals()
    wrapper.unmount()
  })

  it('shows why the hand-off could not start and still offers the feed', async () => {
    firstRun()
    mocks.AgentsAvailable.mockResolvedValue({ available: false, reason: 'no ptyterm on this build.' })
    const wrapper = await mountApp()
    await passHiveStep(wrapper)
    await connectGitHub()
    await wrapper.get('[data-testid="onboarding-permissions-skip"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="onboarding-agent-start"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="onboarding-error"]').text()).toContain('no ptyterm')
    expect(mocks.SetOnboardingCompleted).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="onboarding-agent-start"]').exists()).toBe(true)

    await wrapper.get('[data-testid="onboarding-agent-skip"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)
    expect(mocks.SetOnboardingCompleted).toHaveBeenCalledOnce()

    wrapper.unmount()
  })

  it('skipping the connect step lands on a feed whose empty state points at Integrations', async () => {
    firstRun()
    // A profile that was never seeded has no graph at all.
    mocks.GetFlow.mockResolvedValue({ id: 'default', name: 'Default', enabled: true, nodes: [], wires: [] })
    const { wrapper, router } = await mountAppWithRouter()
    await passHiveStep(wrapper)

    await wrapper.get('[data-testid="onboarding-skip"]').trigger('click')
    await wrapper.get('[data-testid="onboarding-skip-confirm"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="onboarding-permissions-skip"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="onboarding-agent-skip"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)
    expect(mocks.SeedStarterFlow).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="workspace-empty"]').text()).toContain('no account is connected')

    await wrapper.get('[data-testid="workspace-empty-integrations"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')
    expect(router.currentRoute.value.params.section).toBe('integrations')

    wrapper.unmount()
  })

  it('stays on the feed when GitHub disconnects — Integrations is where that is repaired', async () => {
    const wrapper = await mountApp()
    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)

    mocks.Status.mockResolvedValue({ state: 'disconnected', login: '', name: '', avatarUrl: '', message: '' })
    const connection = mocks.On.mock.calls.find(([event]) => event === 'connection:updated')?.[1] as
      ((ev: { data: string }) => void) | undefined
    connection?.({ data: 'github' })
    await flushPromises()

    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Personal')

    wrapper.unmount()
  })

  it('confirms updates in-app and shows install failures', async () => {
    mocks.UpdaterStatus.mockResolvedValue({
      enabled: true,
      available: true,
      currentVersion: '1.2.0',
      latestVersion: '1.3.0',
      notes: '',
    })
    mocks.InstallUpdate.mockRejectedValue(new Error('checksum mismatch'))
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = await mountApp()

    try {
      await wrapper.get('[data-testid="titlebar-update-chip"]').trigger('click')
      expect(document.querySelector('[data-testid="update-confirmation"]')?.textContent).toContain(
        'Download Hive 1.3.0',
      )
      expect(mocks.InstallUpdate).not.toHaveBeenCalled()

      document.querySelector<HTMLButtonElement>('[data-testid="update-confirmation-confirm"]')?.click()
      await flushPromises()

      expect(mocks.InstallUpdate).toHaveBeenCalledOnce()
      expect(document.querySelector('[data-testid="update-confirmation-error"]')?.textContent).toContain(
        'checksum mismatch',
      )
      expect(wrapper.get('[data-testid="toast-title"]').text()).toBe('Could not install the update')
      expect(wrapper.get('[data-testid="toast-body"]').text()).toContain('checksum mismatch')
      expect(wrapper.get('[data-testid="titlebar-update-chip"]').attributes('disabled')).toBeUndefined()
    } finally {
      wrapper.unmount()
      consoleError.mockRestore()
    }
  })

  // The one seam the composable's own tests cannot cover: the Wails event that
  // carries a background failure back to a closed dialog.
  it('turns a failed session create into a toast that reopens the restored form', async () => {
    mocks.FailedSessionDraft.mockResolvedValue({
      repository: 'https://github.com/acme/site.git',
      name: 'fix-crash',
      prompt: 'Fix the crash',
      agent: 'claude',
      itemId: 0,
      failure: {
        reason: 'clone repository: git clone: exec git: exit status 1',
        step: 'Cloning repository...',
        output: 'Clone strategy: full\nCloning repository...',
        cloneStrategy: 'full',
        at: '2026-09-16T10:00:00Z',
      },
    })
    const wrapper = await mountApp()

    try {
      const failed = mocks.On.mock.calls.find(([event]) => event === 'sessions:create-failed')?.[1] as
        ((ev: { data: string }) => void) | undefined
      expect(failed).toBeDefined()
      failed?.({ data: 'fix-crash' })
      await flushPromises()

      expect(wrapper.get('[data-testid="toast-title"]').text()).toContain('fix-crash')
      expect(wrapper.get('[data-testid="toast-body"]').text()).toContain('Cloning repository...')
      expect(wrapper.find('[data-testid="toast-progress"]').exists()).toBe(false)

      const retry = wrapper.findAll('[data-testid="toast-action"]').find((b) => b.text() === 'Retry')
      expect(retry).toBeDefined()
      await retry?.trigger('click')
      await flushPromises()

      const dialog = document.querySelector('[data-testid="new-session-dialog"]')
      expect(dialog).not.toBeNull()
      expect(document.querySelector<HTMLInputElement>('[data-testid="new-session-name"]')?.value).toBe('fix-crash')
      expect(document.querySelector('[data-testid="new-session-failure-reason"]')?.textContent).toContain(
        'exit status 1',
      )
    } finally {
      wrapper.unmount()
    }
  })

  it('registers profile / feed-selection / flow-edit palette commands (not the removed feed-editor ones)', async () => {
    const wrapper = await mountApp()
    const { results, query } = useCommandPalette()
    query.value = ''

    const ids = results.value.map((cmd) => cmd.id)
    expect(ids).toContain('flow:edit')
    expect(ids).toContain('view:trash')
    expect(ids).toContain('feed:personal/desktop')
    expect(ids).toContain('profile:new')
    // Feed/source editing folded into the node drawer — these are gone.
    expect(ids).not.toContain('feed:new')
    expect(ids).not.toContain('feed:edit:desktop')
    expect(ids).not.toContain('feed:edit-config')

    wrapper.unmount()
  })

  it('exposes item selection as a command and selected-item creation as a palette action', async () => {
    mocks.ListByFeed.mockResolvedValue(inboxItems())
    mocks.NewSessionDraft.mockResolvedValue({
      repository: '',
      name: 'inbox-selection-a1b2c3d4',
      prompt: 'Combined context',
    })
    const wrapper = await mountApp()
    const palette = useCommandPalette()
    palette.query.value = ''

    const toggle = palette.results.value.find((candidate) => candidate.id === 'feed.toggle-selection')
    expect(toggle?.title).toBe('Toggle item selection')
    expect(palette.results.value.some((candidate) => candidate.id === 'feed:create-session-from-selection')).toBe(false)

    void toggle!.run()
    await flushPromises()
    expect(wrapper.find('[data-testid="feed-selection-bar"]').exists()).toBe(true)

    const rows = wrapper.findAll('[data-testid="feed-item"]')
    await rows[0]!.trigger('click')
    await rows[1]!.trigger('click')
    await flushPromises()

    const create = palette.results.value.find((candidate) => candidate.id === 'feed:create-session-from-selection')
    expect(create?.title).toBe('Create session from selected items…')
    void create!.run()
    await flushPromises()

    expect(mocks.NewSessionDraft).toHaveBeenCalledWith([1, 2])
    expect(document.querySelector('[data-testid="new-session-dialog"]')).not.toBeNull()

    palette.query.value = ''
    wrapper.unmount()
  })

  // A configured action was reachable from the detail pane's cards and the row
  // menu, but never from the palette. It is grouped under the item it acts on,
  // the way Code groups a session's operations under the session.
  it('offers the selected item\u2019s configured actions, under the item\u2019s own reference', async () => {
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review PR', type: 'shell', inputs: [] }])
    mocks.InvokeAction.mockResolvedValue({ commandId: 7, status: 'completed', stdout: '', stderr: '' })
    mocks.ListByFeed.mockResolvedValue(inboxItems())
    const wrapper = await mountApp()
    await wrapper.findAll('[data-testid="feed-item"]')[0]!.trigger('click')
    await flushPromises()

    const { results, query } = useCommandPalette()
    query.value = ''
    const cmd = results.value.find((candidate) => candidate.id === 'item:action:review')
    expect(cmd?.title).toBe('Review PR')
    expect(cmd?.group).toBe('acme/app #1')

    await cmd!.run()
    await flushPromises()
    expect(mocks.InvokeAction).toHaveBeenCalledWith('review', 1, {})

    query.value = ''
    wrapper.unmount()
  })

  // A launcher is a line of actions.yml that has to become both a palette row
  // and a chord of its own — this is where those two meet the app.
  it('offers a configured launcher in the palette and opens it on the session it is attached to', async () => {
    mocks.PopupLaunchers.mockResolvedValue([
      { id: 'lazygit', label: 'lazygit', icon: 'git-branch', requiresSession: true },
    ])
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    const { results, query } = useCommandPalette()
    query.value = ''
    expect(results.value.map((cmd) => cmd.id)).toContain('launcher.lazygit')

    useKeybindings().addBinding('launcher.lazygit', 'alt+g')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g', altKey: true }))

    const popup = usePopupTerminal()
    expect(popup.visible.value).toBe(true)
    expect(popup.request.value).toEqual({ launcher: 'lazygit', sessionSlug: 'hive-fix-parser' })

    // The chord that opened it puts it away, so quitting the program is not the
    // only way out.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g', altKey: true }))
    expect(popup.visible.value).toBe(false)

    wrapper.unmount()
  })

  // The scratch terminal is a tmux session with no hive record behind it, and a
  // launcher reaches it on the same route param every other row uses — what a
  // cwd-less launcher needs is a pane to read, not a checkout (ADR
  // a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is).
  it('opens a session-scoped launcher on the scratch terminal', async () => {
    mocks.PopupLaunchers.mockResolvedValue([
      { id: 'lazygit', label: 'lazygit', icon: 'git-branch', requiresSession: true },
    ])
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/Scratch')
    await flushPromises()

    const { results, query } = useCommandPalette()
    query.value = ''
    expect(results.value.map((cmd) => cmd.id)).toContain('launcher.lazygit')

    useKeybindings().addBinding('launcher.lazygit', 'alt+g')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g', altKey: true }))

    const popup = usePopupTerminal()
    expect(popup.visible.value).toBe(true)
    expect(popup.request.value).toEqual({ launcher: 'lazygit', sessionSlug: 'Scratch' })

    wrapper.unmount()
  })

  // The bug this is here for: `lazygit` opened from the feed used to start in
  // the home directory and present a failed TUI. A launcher that runs where a
  // terminal is is not offered where there is no terminal, and its chord is not
  // dispatched there either (ADR quick-terminal-launchers-are-session-scoped).
  it('withholds a session-scoped launcher with no terminal attached, from the palette and from its chord', async () => {
    mocks.PopupLaunchers.mockResolvedValue([
      { id: 'lazygit', label: 'lazygit', icon: 'git-branch', requiresSession: true },
    ])
    const { wrapper, router } = await mountAppWithRouter()
    useKeybindings().addBinding('launcher.lazygit', 'alt+g')

    const { results, query } = useCommandPalette()
    const popup = usePopupTerminal()
    query.value = ''

    // On the feed: no session, so nothing to run in.
    expect(results.value.map((cmd) => cmd.id)).not.toContain('launcher.lazygit')
    const onFeed = new KeyboardEvent('keydown', { key: 'g', altKey: true, cancelable: true })
    window.dispatchEvent(onFeed)
    expect(popup.visible.value).toBe(false)
    expect(onFeed.defaultPrevented).toBe(false)

    // Terminal mode with nothing attached is the session picker, and a launcher
    // has no more to work with there than it does on the feed.
    await router.push('/terminal')
    await flushPromises()
    expect(results.value.map((cmd) => cmd.id)).not.toContain('launcher.lazygit')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g', altKey: true }))
    expect(popup.visible.value).toBe(false)

    wrapper.unmount()
  })

  // The palette is scoped to where the user stands, but only for what is
  // actually tied to the hub view: the feed-context catalog command and the
  // profile-bound flow/action rows drop out of the Code view, while the hub's
  // own Go-to objects (feeds, Trash, profiles, themes, settings) now reach
  // across every mode (#306) — their run()s already land in the hub from
  // anywhere. Terminal-only commands still drop out of the hub, while the
  // sidebar command follows whichever app mode owns the visible left panel.
  it('filters palette rows by mode while keeping the sidebar command with every app mode', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    const { results, query } = useCommandPalette()
    query.value = ''

    let ids = results.value.map((cmd) => cmd.id)
    expect(ids).toContain('feed:personal/desktop')
    expect(ids).toContain('feed.refresh')
    expect(ids.filter((id) => id.startsWith('theme:')).length).toBeGreaterThan(0)
    expect(ids).not.toContain('terminal.focus-sidebar')
    expect(ids).toContain('terminal.toggle-sidebar')
    expect(ids).toContain('mode:terminal')
    expect(ids).not.toContain('mode:hub')

    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    ids = results.value.map((cmd) => cmd.id)
    // Still present: the hub's Go-to objects, reachable from Code now too.
    expect(ids).toContain('feed:personal/desktop')
    expect(ids).toContain('view:trash')
    expect(ids).toContain('profile:personal')
    expect(ids.filter((id) => id.startsWith('theme:')).length).toBeGreaterThan(0)
    expect(ids.some((id) => id.startsWith('settings:'))).toBe(true)
    expect(ids).toContain('mode:hub')
    // Still absent: the feed-context catalog command and the hub-only actions.
    expect(ids).not.toContain('feed.refresh')
    expect(ids).not.toContain('flow:edit')
    expect(ids).not.toContain('profile:new')
    expect(ids.some((id) => id.startsWith('item:action:'))).toBe(false)
    expect(ids).not.toContain('mode:terminal')
    expect(ids).toContain('terminal.focus-sidebar')
    expect(ids).toContain('terminal.toggle-sidebar')
    expect(ids).toContain('session.new')

    await router.push('/settings/integrations')
    await flushPromises()
    ids = results.value.map((cmd) => cmd.id)
    expect(ids).not.toContain('terminal.toggle-sidebar')

    wrapper.unmount()
  })

  // stepSequence (useKeybindings) decides what a combo means; App.vue only
  // stores the pending state, arms/cancels the deferred timer, and dispatches
  // through the same gate an ordinary chord uses. These bind an existing
  // global command to a synthetic sequence rather than the catalog's real
  // ones, so the assertions stay isolated from changes to the shipped
  // defaults.
  describe('keyboard sequences', () => {
    it('dispatches the bound command once a two-step sequence completes', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'g z')
      const { open: paletteOpen } = useCommandPalette()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value?.steps).toEqual(['g'])
      expect(paletteOpen.value).toBe(false)

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'z' }))
      expect(paletteOpen.value).toBe(true)
      expect(kb.pendingSequence.value).toBeNull()

      paletteOpen.value = false
      wrapper.unmount()
    })

    it('swallows an unmatched bare key mid-sequence: default prevented, nothing dispatched', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'g z')
      const { open: paletteOpen } = useCommandPalette()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      const stray = new KeyboardEvent('keydown', { key: 'x', cancelable: true })
      window.dispatchEvent(stray)

      expect(stray.defaultPrevented).toBe(true)
      expect(paletteOpen.value).toBe(false)
      expect(kb.pendingSequence.value).toBeNull()

      wrapper.unmount()
    })

    it('falls a mod-carrying chord mid-sequence through to its own binding', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'g z') // any prefix binding, just to get a sequence pending
      const { open: paletteOpen } = useCommandPalette()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      // mod+k is palette.toggle's own default binding, unrelated to 'g z'.
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))

      expect(paletteOpen.value).toBe(true)
      expect(kb.pendingSequence.value).toBeNull()

      paletteOpen.value = false
      wrapper.unmount()
    })

    it('clears the pending sequence on Escape, so finishing it afterward does nothing', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'g z')
      const { open: paletteOpen } = useCommandPalette()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value).not.toBeNull()

      const esc = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true })
      window.dispatchEvent(esc)
      expect(esc.defaultPrevented).toBe(true)
      expect(kb.pendingSequence.value).toBeNull()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'z' }))
      expect(paletteOpen.value).toBe(false)

      wrapper.unmount()
    })

    it('clears the pending sequence when focus moves into a terminal pane', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'g z')

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value).not.toBeNull()

      const pane = document.createElement('div')
      pane.setAttribute('data-terminal-input-scope', '')
      document.body.append(pane)
      pane.dispatchEvent(new FocusEvent('focusin', { bubbles: true }))

      expect(kb.pendingSequence.value).toBeNull()

      pane.remove()
      wrapper.unmount()
    })

    // Regression: a mouse click into a text field is a focusin with no
    // intervening keystroke, so onWindowFocusIn is the only thing that can
    // catch it. Before the fix it reset only for a terminal target, so a
    // pending sequence survived the click and hijacked the field's next
    // keystroke (dispatching it as the sequence's continuation, or
    // swallowing it as an unmatched key) instead of letting it type.
    it('clears the pending sequence when focus moves into an editable target, so typing continues normally', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'g z')
      const { open: paletteOpen } = useCommandPalette()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value).not.toBeNull()

      const input = document.createElement('input')
      document.body.append(input)
      input.dispatchEvent(new FocusEvent('focusin', { bubbles: true }))

      expect(kb.pendingSequence.value).toBeNull()

      const event = new KeyboardEvent('keydown', { key: 'z', bubbles: true, cancelable: true })
      input.dispatchEvent(event)

      expect(paletteOpen.value).toBe(false)
      expect(event.defaultPrevented).toBe(false)
      expect(kb.pendingSequence.value).toBeNull()

      input.remove()
      wrapper.unmount()
    })

    // A discarded sequence start must still fall through to the exact
    // binding on the same combo (Zed's prefix rule: a bound-elsewhere combo
    // stays fully functional even though it also prefixes something longer).
    // Regression for a bug where the suppressed start returned outright,
    // dropping the combo's own binding in an editable field.
    it('dispatches a combo that is also a sequence prefix in an editable field, starting no sequence', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'mod+e')
      kb.addBinding('report.bundle', 'mod+e x')
      const { open: paletteOpen } = useCommandPalette()

      // A plain input appended straight to the document, like the terminal
      // pane fixture below — the mounted tree isn't attached to the document,
      // so a bubbling keydown dispatched on it would never reach the window
      // listener onGlobalKeydown runs on.
      const input = document.createElement('input')
      document.body.append(input)

      const event = new KeyboardEvent('keydown', { key: 'e', metaKey: true, bubbles: true, cancelable: true })
      input.dispatchEvent(event)

      expect(paletteOpen.value).toBe(true)
      expect(kb.pendingSequence.value).toBeNull()

      paletteOpen.value = false
      input.remove()
      wrapper.unmount()
    })

    // End-to-end over the real catalog: the default 'g i' binding reaches
    // runMap's view.go-inbox entry.
    it('switches to the Inbox view on the g i sequence', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      await router.push('/terminal/hive-fix-parser')
      await flushPromises()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i' }))
      await flushPromises()

      expect(router.currentRoute.value.name).toBe('feed')

      wrapper.unmount()
    })

    // Zed's prefix rule: a step that is both a complete binding and a prefix of
    // another defers rather than firing immediately, so a continuation still
    // gets its chance. The catalog has no such dual binding today, so this adds
    // one beside the default 'g i'/'g c'/... prefixes rather than relying on one.
    it('defers a step that is also a complete binding, firing it on the timeout if nothing continues it', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('palette.toggle', 'g')
      const { open: paletteOpen } = useCommandPalette()

      vi.useFakeTimers()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value?.steps).toEqual(['g'])
      expect(paletteOpen.value).toBe(false) // deferred, not dispatched yet

      vi.advanceTimersByTime(SEQUENCE_TIMEOUT_MS)
      expect(paletteOpen.value).toBe(true)
      expect(kb.pendingSequence.value).toBeNull()

      paletteOpen.value = false
      wrapper.unmount()
    })

    it('cancels the deferred timer when a continuation arrives first, so the deferred binding never also fires', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('report.bundle', 'g') // dual bound+prefix, same as above
      const report = useReportDialog()

      vi.useFakeTimers()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i' })) // completes 'g i' -> view.go-inbox
      expect(kb.pendingSequence.value).toBeNull()
      expect(report.open.value).toBe(false)

      vi.advanceTimersByTime(SEQUENCE_TIMEOUT_MS)
      expect(report.open.value).toBe(false) // the cancelled timer does not also fire

      wrapper.unmount()
    })

    it('re-applies the overlay check when the deferred timer fires, not the check that held when it was armed', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      kb.addBinding('view.go-code', 'g')
      const { openBundleDialog: openReport } = useReportDialog()

      vi.useFakeTimers()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value?.steps).toEqual(['g'])

      // A different overlay opens (a mouse click, say) while the timer is
      // still pending — nothing about that clears pendingSequence, so the
      // suppression has to come from the fire-time check instead.
      openReport()
      vi.advanceTimersByTime(SEQUENCE_TIMEOUT_MS)

      expect(terminalOnScreen(wrapper)).toBe(false) // view.go-code never ran

      wrapper.unmount()
    })

    it('does not start a pending sequence from an editable target', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      const input = document.createElement('input')
      document.body.append(input)

      const event = new KeyboardEvent('keydown', { key: 'g', bubbles: true, cancelable: true })
      input.dispatchEvent(event)

      expect(kb.pendingSequence.value).toBeNull()
      expect(event.defaultPrevented).toBe(false) // typing proceeds normally

      input.remove()
      wrapper.unmount()
    })

    it('does not start a pending sequence while an overlay is open', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()
      const { openBundleDialog: openReport, close: closeReport } = useReportDialog()

      openReport()
      await flushPromises()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value).toBeNull()

      closeReport()
      wrapper.unmount()
    })

    // Distinct from the focusin case above: a keydown can target a pane that
    // already has focus, with no intervening focus change to catch.
    it('clears the pending sequence on any keydown that targets a focused terminal pane', async () => {
      const wrapper = await mountApp()
      const kb = useKeybindings()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value).not.toBeNull()

      const pane = focusedPane()
      pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'x', bubbles: true }))
      expect(kb.pendingSequence.value).toBeNull()

      pane.remove()
      wrapper.unmount()
    })

    // The accepted asymmetry (ADR keybindings-are-chord-sequences-not-a-leader-key):
    // a sequence cannot start while an overlay owns the screen, so 'g' then 't'
    // never reaches tasks.toggle once Tasks is already open — only the chord,
    // which is an ordinary single-combo dispatch, can close it again.
    it('pins the overlay-toggle asymmetry: g t cannot close Tasks, mod+shift+t can', async () => {
      const { wrapper } = await mountAppWithRouter()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
      await flushPromises()
      expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 't' }))
      await flushPromises()
      expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull() // still open

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
      await flushPromises()
      expect(document.querySelector('[data-testid="tasks-overlay"]')).toBeNull() // the chord does close it

      wrapper.unmount()
    })

    it('steps the terminal text size from a focused pane, and resets it', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      await router.push('/terminal/hive-fix-parser')
      await flushPromises()

      const { px } = useTerminalFont()
      const pane = focusedPane()
      const press = async (init: KeyboardEventInit): Promise<void> => {
        pane.dispatchEvent(new KeyboardEvent('keydown', { metaKey: true, bubbles: true, ...init }))
        await flushPromises()
      }

      await press({ key: '=' })
      expect(px.value).toBe(defaultTerminalFontSizePx + 2)

      // The bare plus of a layout with its own plus key. The shifted spelling
      // ⌘+ produces is macOS-only, so catalog.spec pins that one.
      await press({ key: '+' })
      expect(px.value).toBe(defaultTerminalFontSizePx + 4)

      await press({ key: '-' })
      expect(px.value).toBe(defaultTerminalFontSizePx + 2)

      await press({ key: '0' })
      expect(px.value).toBe(defaultTerminalFontSizePx)

      pane.remove()
      wrapper.unmount()
    })

    it('steps the text size from the pop-up terminal over the feed', async () => {
      const { wrapper } = await mountAppWithRouter()
      usePopupTerminal().show()
      await flushPromises()

      const { px } = useTerminalFont()
      const pane = focusedPane()
      pane.dispatchEvent(new KeyboardEvent('keydown', { key: '=', metaKey: true, bubbles: true }))
      await flushPromises()

      expect(px.value).toBe(defaultTerminalFontSizePx + 2)
      pane.remove()
      wrapper.unmount()
    })

    it.each([
      ['the feed', '/'],
      ['the session picker', '/terminal'],
    ])('leaves the text size alone on %s, where no terminal is drawn', async (_where, path) => {
      const { wrapper, router } = await mountAppWithRouter()
      await router.push(path)
      await flushPromises()

      const { px } = useTerminalFont()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: '=', metaKey: true }))
      await flushPromises()

      expect(px.value).toBe(defaultTerminalFontSizePx)
      wrapper.unmount()
    })

    // Regression: the pierce block (terminal.focus-sidebar and friends)
    // dispatches via runCommand before stepSequence ever runs, so it used to
    // leave an unrelated pending sequence (and its hint pill) stranded.
    it('clears the pending sequence when a pierced command dispatches over a focused terminal', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      await router.push('/terminal/hive-fix-parser')
      await flushPromises()

      const kb = useKeybindings()
      const { focusTree } = stubTerminalTree()
      const pane = focusedPane()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
      expect(kb.pendingSequence.value).not.toBeNull()

      const event = new KeyboardEvent('keydown', { key: 'ArrowLeft', metaKey: true, bubbles: true, cancelable: true })
      pane.dispatchEvent(event)
      await flushPromises()

      expect(focusTree).toHaveBeenCalled() // confirms the pierce path actually fired
      expect(kb.pendingSequence.value).toBeNull()

      setTerminalTreeHandles(null)
      pane.remove()
      wrapper.unmount()
    })
  })

  // useAppPaletteRows registers Go-to rows at the App level, off the same
  // module singletons the sidebar trees read — so they exist independent of
  // whichever mode happens to be mounted, and running one dispatches straight
  // through the router rather than through a mode's own local state.
  describe('global Go-to rows (useAppPaletteRows)', () => {
    it('runs a session attach row by pushing /terminal/:slug', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      vi.mocked(ListSessions).mockResolvedValueOnce([
        {
          id: '1',
          name: 'fix the parser',
          slug: 'hive-fix-parser',
          repo: 'hay-kot/hive',
          state: 'active',
          canvasOwner: 'hay-kot/hive',
        },
      ])
      await useTerminalSessions().reload()

      const { results, query } = useCommandPalette()
      query.value = ''
      const cmd = results.value.find((candidate) => candidate.id === 'terminal:attach:hive-fix-parser')
      expect(cmd?.title).toBe('fix the parser')

      await cmd!.run()
      await flushPromises()

      expect(router.currentRoute.value.name).toBe('terminal')
      expect(router.currentRoute.value.params.slug).toBe('hive-fix-parser')

      wrapper.unmount()
    })

    // The projection now survives a trip back to the hub (TerminalMode.spec's
    // "survives a trip back to the hub"), so a populated projection while the
    // route is nowhere near /terminal is a state a real attach actually
    // leaves behind — not a synthetic one, which is what let this row run
    // into an attach that had never happened. Seeding it directly still
    // isolates the App-level push from TerminalMode's own attach machinery;
    // that the push actually selects the window on the pooled client is
    // TerminalMode.spec's "selects the window a same-slug ?window push
    // names on the pooled client".
    it('runs a window row by pushing /terminal/:slug with ?window=, from outside Code entirely', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      setAttachedTerminalWindows({
        slug: 'hive-fix-parser',
        name: 'fix the parser',
        windows: [
          { windowId: '@1', name: 'agent', active: true },
          { windowId: '@2', name: 'shell', active: false },
        ],
      })
      expect(terminalOnScreen(wrapper)).toBe(false)

      const { results, query } = useCommandPalette()
      query.value = ''
      const cmd = results.value.find((candidate) => candidate.id === 'terminal:window:@2')
      expect(cmd?.title).toBe('shell')

      await cmd!.run()
      await flushPromises()

      expect(router.currentRoute.value.name).toBe('terminal')
      expect(router.currentRoute.value.params.slug).toBe('hive-fix-parser')
      expect(router.currentRoute.value.query.window).toBe('@2')

      wrapper.unmount()
    })

    it('selects a feed row from the Code view and lands on the feed route', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      await router.push('/terminal/hive-fix-parser')
      await flushPromises()

      const { results, query } = useCommandPalette()
      query.value = ''
      const cmd = results.value.find((candidate) => candidate.id === 'feed:personal/desktop')
      expect(cmd).toBeDefined()

      await cmd!.run()
      await flushPromises()

      expect(router.currentRoute.value.name).toBe('feed')

      wrapper.unmount()
    })

    it('runs the Hive CLI settings row by pushing application-settings with the section param', async () => {
      const { wrapper, router } = await mountAppWithRouter()

      const { results, query } = useCommandPalette()
      query.value = ''
      const cmd = results.value.find((candidate) => candidate.id === 'settings:hive')
      expect(cmd).toMatchObject({
        title: 'Hive CLI',
        group: 'Settings',
        scope: 'goto',
        kind: 'settings',
      })

      await cmd!.run()
      await flushPromises()

      expect(router.currentRoute.value.name).toBe('application-settings')
      expect(router.currentRoute.value.params.section).toBe('hive')

      wrapper.unmount()
    })

    it('runs a Keys-scope row by requesting the editor filter and routing to Settings › Keyboard', async () => {
      const { wrapper, router } = await mountAppWithRouter()

      const palette = useCommandPalette()
      palette.query.value = ''
      palette.scope.value = 'keys'
      const cmd = palette.results.value.find((candidate) => candidate.id === 'feed.next')
      expect(cmd?.title).toBe('Next item')

      // requestedEditorFilter is set synchronously, before the router push
      // (and any settings pane it mounts) has had a chance to consume it.
      void cmd!.run()
      expect(requestedEditorFilter.value).toBe('Next item')

      await flushPromises()
      expect(router.currentRoute.value.name).toBe('application-settings')
      expect(router.currentRoute.value.params.section).toBe('keybindings')

      palette.scope.value = 'all'
      wrapper.unmount()
    })

    // KeymapRow carries the catalog's keywords through to the Keys-scope row
    // now, so a query matching a synonym finds the command even though the
    // synonym never appears in its title or group.
    it('matches a Keys-scope row by a keyword synonym rather than only its title', async () => {
      const { wrapper } = await mountAppWithRouter()

      const palette = useCommandPalette()
      palette.scope.value = 'keys'
      palette.query.value = 'catch up'
      const cmd = palette.results.value.find((candidate) => candidate.id === 'feed.mark-all-read')
      expect(cmd?.title).toBe('Mark all as read')

      palette.query.value = ''
      palette.scope.value = 'all'
      wrapper.unmount()
    })

    it('filters the sigil legend to scopes whose tab is visible', async () => {
      const { wrapper } = await mountAppWithRouter()

      const palette = useCommandPalette()
      palette.query.value = ''
      palette.scope.value = 'keys'

      // No shell escape is registered outside the Code view, so the ! legend
      // row — whose run would strand the palette in a scope with no tab and
      // no possible rows — is hidden along with its tab.
      const legendIds = palette.results.value.filter((cmd) => cmd.group === 'Sigils').map((cmd) => cmd.id)
      expect(legendIds).toEqual(['keys:sigil:goto', 'keys:sigil:actions', 'keys:sigil:keys'])

      palette.scope.value = 'all'
      wrapper.unmount()
    })

    // The stub only seeds recents, so useAgentWorkspaces().workspaces stays
    // empty — the dir → name join has nothing to match, and the group falls
    // back to the raw dir key rather than a display name.
    it('lists a chat row, pushes its agents route, and focuses the chat pane', async () => {
      mocks.AgentsAvailable.mockResolvedValue({ available: true, reason: '' })
      mocks.agentsClient = {
        allSessions: vi.fn().mockResolvedValue([
          {
            id: 42,
            workspace: 'my-workspace',
            name: 'Chat about the bug',
            agent: 'claude',
            lastOpenedAt: 0,
            slug: 'chat-42',
            terminalId: '',
            windowId: '',
            paneId: '',
            cols: 0,
            rows: 0,
            resumeAttempted: false,
            notice: '',
            scheduleId: '',
          },
        ]),
      } as unknown as AgentWorkspacesClient
      const { wrapper, router } = await mountAppWithRouter()
      await useAgentSessionsAll().reload()

      // Mount the Chats mode once, then replace its handles while it is hidden.
      // Returning through the palette reuses that mounted pane.
      await router.push({ name: 'agents', params: { workspace: 'my-workspace' }, query: { chat: '42' } })
      await flushPromises()
      await router.push('/feed')
      await flushPromises()
      const focusPane = vi.fn()
      setAgentsTreeHandles({ focusList: vi.fn(), focusFilter: vi.fn(), focusPane })

      const palette = useCommandPalette()
      palette.query.value = ''
      const cmd = palette.results.value.find((candidate) => candidate.id === 'chat:42')
      expect(cmd?.title).toBe('Chat about the bug')
      expect(cmd?.group).toBe('my-workspace')

      const push = vi.spyOn(router, 'push')
      await palette.run(cmd!)
      await flushPromises()

      // Asserted on the push, not the settled route: this chat has no terminal,
      // so the Chats view drops ?chat once its own resume check resolves.
      expect(push).toHaveBeenCalledWith({
        name: 'agents',
        params: { workspace: 'my-workspace' },
        query: { chat: '42' },
      })
      expect(router.currentRoute.value.name).toBe('agents')
      expect(router.currentRoute.value.params.workspace).toBe('my-workspace')
      expect(focusPane).toHaveBeenCalledTimes(1)

      setAgentsTreeHandles(null)
      wrapper.unmount()
    })

    // session.workspace is a directory key ("my-workspace"), not the display
    // name a user picked ("Travel") — this is the #338-adjacent bug the join
    // in useAppPaletteRows fixes: once useAgentWorkspaces().workspaces knows
    // the dir, the chat row's group resolves to the workspace's real name.
    it('groups a chat row under the workspace display name once the workspaces list has a matching dir', async () => {
      mocks.AgentsAvailable.mockResolvedValue({ available: true, reason: '' })
      mocks.agentsClient = {
        workspaces: vi.fn().mockResolvedValue({
          root: '',
          rootProblem: '',
          available: true,
          error: '',
          editor: { command: '', title: '' },
          presets: [],
          workspaces: [
            {
              dir: 'my-workspace',
              name: 'Travel',
              command: 'claude',
              danger: false,
              mcps: [],
              skills: [],
              schedules: [],
              problem: '',
              notice: '',
            },
          ],
        }),
        allSessions: vi.fn().mockResolvedValue([
          {
            id: 42,
            workspace: 'my-workspace',
            name: 'Chat about the bug',
            agent: 'claude',
            lastOpenedAt: 0,
            slug: 'chat-42',
            terminalId: '',
            windowId: '',
            paneId: '',
            cols: 0,
            rows: 0,
            resumeAttempted: false,
            notice: '',
            scheduleId: '',
          },
        ]),
      } as unknown as AgentWorkspacesClient
      const { wrapper } = await mountAppWithRouter()
      await useAgentWorkspaces().reloadWorkspaces()
      await useAgentSessionsAll().reload()

      const { results, query } = useCommandPalette()
      query.value = ''
      const cmd = results.value.find((candidate) => candidate.id === 'chat:42')
      expect(cmd?.title).toBe('Chat about the bug')
      expect(cmd?.group).toBe('Travel')

      wrapper.unmount()
    })

    // TerminalMode never mounts on the feed route (it is mount-on-first-visit),
    // so ListSessions and the Agents probe only fire here through this watch —
    // proof the reload is the palette's own doing, not a side effect of some
    // other component being on screen.
    it('reloads terminal sessions and chat recents when the palette opens', async () => {
      const { wrapper } = await mountAppWithRouter()
      vi.mocked(ListSessions).mockClear()
      mocks.AgentsAvailable.mockClear()

      const palette = useCommandPalette()
      palette.toggle()
      await flushPromises()

      expect(ListSessions).toHaveBeenCalled()
      expect(mocks.AgentsAvailable).toHaveBeenCalled()

      palette.toggle()
      wrapper.unmount()
    })
  })

  // view.focus-search is one command answering `/` on two unrelated surfaces,
  // so a visible row for it would no-op wherever the other surface is on
  // screen — these named rows stand in per surface, gated the same way the
  // surface's own commands are, and both dispatch the same command.
  describe('view.focus-search named rows', () => {
    it('offers "Search items…" on the feed, carrying the / hint, and dispatches into the search box', async () => {
      const { wrapper } = await mountAppWithRouter()
      const input = wrapper.get('[data-testid="feed-search"]').element as HTMLInputElement
      const select = vi.spyOn(input, 'select')

      const { results, query } = useCommandPalette()
      query.value = ''
      expect(results.value.some((cmd) => cmd.id === 'view.focus-search:terminal')).toBe(false)
      const row = results.value.find((cmd) => cmd.id === 'view.focus-search:feed')
      expect(row?.title).toBe('Search items…')
      expect(row?.hint).toBe(formatCombo('/'))

      await row!.run()
      await flushPromises()
      expect(select).toHaveBeenCalled()

      wrapper.unmount()
    })

    it('offers "Filter workspaces" in Chats, carrying the / hint, and dispatches into the workspace filter', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      await router.push('/workspaces')
      await vi.waitFor(() => expect(agentsOnScreen(wrapper)).toBe(true))
      await flushPromises()
      const focusFilter = vi.fn()
      setAgentsTreeHandles({ focusList: vi.fn(), focusFilter, focusPane: vi.fn() })

      const { results, query } = useCommandPalette()
      query.value = ''
      expect(results.value.some((cmd) => cmd.id === 'view.focus-search:terminal')).toBe(false)
      const row = results.value.find((cmd) => cmd.id === 'view.focus-search:agents')
      expect(row?.title).toBe('Filter workspaces')
      expect(row?.hint).toBe(formatCombo('/'))

      await row!.run()
      await flushPromises()
      expect(focusFilter).toHaveBeenCalled()

      setAgentsTreeHandles(null)
      wrapper.unmount()
    })

    it('offers "Filter sessions" in Code, carrying the / hint, and dispatches into the session filter', async () => {
      const { wrapper, router } = await mountAppWithRouter()
      await router.push('/terminal/hive-fix-parser')
      await flushPromises()
      const { focusFilter } = stubTerminalTree()

      const { results, query } = useCommandPalette()
      query.value = ''
      expect(results.value.some((cmd) => cmd.id === 'view.focus-search:feed')).toBe(false)
      const row = results.value.find((cmd) => cmd.id === 'view.focus-search:terminal')
      expect(row?.title).toBe('Filter sessions')
      expect(row?.hint).toBe(formatCombo('/'))

      await row!.run()
      await flushPromises()
      expect(focusFilter).toHaveBeenCalled()

      setTerminalTreeHandles(null)
      wrapper.unmount()
    })
  })

  // A launcher pinned to a directory carries the context it needs in the
  // catalog, so it stays reachable from anywhere — the scope rule is about the
  // ones that resolve their directory from the session.
  it('keeps a launcher with its own working directory reachable off a session', async () => {
    mocks.PopupLaunchers.mockResolvedValue([
      { id: 'dotfiles', label: 'Edit dotfiles', icon: 'folder', requiresSession: false },
    ])
    const wrapper = await mountApp()

    const { results, query } = useCommandPalette()
    query.value = ''
    expect(results.value.map((cmd) => cmd.id)).toContain('launcher.dotfiles')

    useKeybindings().addBinding('launcher.dotfiles', 'alt+d')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'd', altKey: true }))

    const popup = usePopupTerminal()
    expect(popup.visible.value).toBe(true)
    expect(popup.request.value).toEqual({ launcher: 'dotfiles', sessionSlug: undefined })

    wrapper.unmount()
  })

  it('opens the flows canvas from the sidebar and exits via the profile rail, keeping the rail mounted', async () => {
    const wrapper = await mountApp()

    // Feed view first: sidebar present, no flows canvas.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    // Flows canvas is up; the spaces rail stays mounted as the way back.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(true)
    const getFlowCallsBeforeExit = mocks.GetFlow.mock.calls.length

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    // Back to the feed view.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)
    expect(mocks.GetFlow.mock.calls.length).toBeGreaterThan(getFlowCallsBeforeExit)

    wrapper.unmount()
  })

  it('renames the active profile from profile settings', async () => {
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="sidebar-open-settings"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="profile-settings-name"]').setValue('Team')
    await wrapper.get('[data-testid="profile-settings-view"] form').trigger('submit')
    await flushPromises()

    expect(mocks.RenameFlow).toHaveBeenCalledWith('personal', 'Team')
    expect((wrapper.get('[data-testid="profile-settings-name"]').element as HTMLInputElement).value).toBe('Team')

    wrapper.unmount()
  })

  it('collapses and restores the feed sidebar from the title-bar toggle', async () => {
    const wrapper = await mountApp()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    await wrapper.find('[data-testid="titlebar-toggle-sidebar"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(false)
    expect(localStorage.getItem('hive.panel.sidebar.collapsed')).toBe('true')

    await wrapper.find('[data-testid="titlebar-toggle-sidebar"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)
    expect(localStorage.getItem('hive.panel.sidebar.collapsed')).toBe('false')

    wrapper.unmount()
  })

  it('collapses and restores the detail preview from the title-bar toggle and the p key', async () => {
    const wrapper = await mountApp()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(true)

    await wrapper.find('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)
    expect(localStorage.getItem('hive.panel.detailpane.collapsed')).toBe('true')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'p' }))
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('reopens the collapsed preview on a double-click, not on the click that selects', async () => {
    mocks.ListByFeed.mockResolvedValue(inboxItems())
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    // The first click of the gesture selects, and must leave the pane shut.
    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="feed-item"]')[1]!.classes()).toContain('selected')
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('dblclick')
    await flushPromises()
    expect(wrapper.get('[data-testid="detail-pane"] h1').text()).toBe('Second')

    // Double-clicking the row that is already selected reopens too — the
    // gesture is the request to read it, not a selection change.
    await wrapper.get('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('dblclick')
    await flushPromises()
    expect(wrapper.get('[data-testid="detail-pane"] h1').text()).toBe('Second')

    wrapper.unmount()
  })

  it('navigates the feed by keyboard silently, and reopens the preview on the row it activates', async () => {
    mocks.ListByFeed.mockResolvedValue(inboxItems())
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
    await flushPromises()
    expect(wrapper.findAll('[data-testid="feed-item"]')[1]!.classes()).toContain('selected')
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('keydown.enter')
    await flushPromises()
    expect(wrapper.get('[data-testid="detail-pane"] h1').text()).toBe('Second')

    wrapper.unmount()
  })

  it('leaves the collapsed preview shut for row controls and the list header menus', async () => {
    mocks.ListByFeed.mockResolvedValue(inboxItems())
    mocks.SetUnread.mockImplementation(async (id: number, revision: number, unread: boolean) => ({
      ...inboxItems().find((item) => item.id === id)!,
      revision: revision + 1,
      unread,
    }))
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()

    const row = () => wrapper.findAll('[data-testid="feed-item"]')[1]!
    await row().get('[data-testid="row-archive"]').trigger('click')
    await flushPromises()
    expect(mocks.ToggleArchived).toHaveBeenCalledWith(2, 1)
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    await row().get('[data-testid="row-menu-toggle"]').trigger('click')
    await flushPromises()
    await row().get('[data-testid="menu-toggle-read"]').trigger('click')
    await flushPromises()
    expect(mocks.SetUnread).toHaveBeenCalledWith(2, 1, true)
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="view-sort-oldest"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    // A double-click landing inside the hover pill is aimed at its buttons, so
    // it must not reach the row underneath either.
    await row().get('[data-testid="row-hover-actions"]').trigger('dblclick')
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('persists the reopen as the remembered last state', async () => {
    mocks.ListByFeed.mockResolvedValue(inboxItems())
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()
    expect(localStorage.getItem('hive.panel.detailpane.collapsed')).toBe('true')

    await wrapper.findAll('[data-testid="feed-item"]')[0]!.trigger('dblclick')
    await flushPromises()
    expect(localStorage.getItem('hive.panel.detailpane.collapsed')).toBe('false')

    wrapper.unmount()
  })

  it('starts collapsed when that is the persisted last state', async () => {
    localStorage.setItem('hive.panel.detailpane.collapsed', 'true')
    mocks.ListByFeed.mockResolvedValue(inboxItems())
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('opens profile settings from the sidebar gear and application settings from the rail', async () => {
    const wrapper = await mountApp()

    await wrapper.find('[data-testid="sidebar-open-settings"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="profile-settings-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(false)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="settings-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profile-settings-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('uses route history for settings pages and categories', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')
    expect(wrapper.find('[data-testid="settings-general"]').exists()).toBe(true)

    await wrapper.find('[data-testid="settings-category-integrations"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('integrations')
    expect(wrapper.find('[data-testid="settings-integrations"]').exists()).toBe(true)

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')
    expect(router.currentRoute.value.params.section).toBe('')
    expect(wrapper.find('[data-testid="settings-general"]').exists()).toBe(true)

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('feed')
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('uses mouse back and forward buttons for route history', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')

    const backDown = new MouseEvent('mousedown', { button: 3, cancelable: true })
    window.dispatchEvent(backDown)
    expect(backDown.defaultPrevented).toBe(true)

    const backUp = new MouseEvent('mouseup', { button: 3, cancelable: true })
    window.dispatchEvent(backUp)
    await flushPromises()
    expect(backUp.defaultPrevented).toBe(true)
    expect(router.currentRoute.value.name).toBe('feed')

    const forwardUp = new MouseEvent('mouseup', { button: 4, cancelable: true })
    window.dispatchEvent(forwardUp)
    await flushPromises()
    expect(forwardUp.defaultPrevented).toBe(true)
    expect(router.currentRoute.value.name).toBe('application-settings')

    wrapper.unmount()
  })

  it('suppresses Backspace history navigation outside editable fields', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()
    const routeBefore = router.currentRoute.value.fullPath

    const backspace = new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true, cancelable: true })
    window.dispatchEvent(backspace)
    await flushPromises()

    expect(backspace.defaultPrevented).toBe(true)
    expect(router.currentRoute.value.fullPath).toBe(routeBefore)

    wrapper.unmount()
  })

  it('allows Backspace to edit text inputs', async () => {
    const { wrapper } = await mountAppWithRouter()
    const search = wrapper.get('[data-testid="feed-search"]').element
    const backspace = new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true, cancelable: true })

    search.dispatchEvent(backspace)

    expect(backspace.defaultPrevented).toBe(false)
    wrapper.unmount()
  })

  it('renders notifications settings from its deep link', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/settings/notifications')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()

    expect(router.currentRoute.value.params.section).toBe('notifications')
    expect(wrapper.find('[data-testid="notification-settings"]').exists()).toBe(true)
    wrapper.unmount()
  })

  // Routing a section is not the same as reaching it: App resolves :section
  // itself, and a section it does not recognize silently renders the default
  // pane. Clicking each nav entry is the only check that covers both halves.
  it.each(applicationSettingsSections)('navigates to the %s settings section from its nav entry', async (section) => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/settings')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()

    await wrapper.get(`[data-testid="settings-category-${section}"]`).trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.params.section).toBe(section)
    expect(
      wrapper.get(`[data-testid="settings-category-${section}"]`).attributes('aria-current'),
      `the ${section} nav entry is not marked current — App resolved :section to another pane`,
    ).toBe('true')
    wrapper.unmount()
  })

  it('renders developer tools from its deep link', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/dev')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })

    try {
      await flushPromises()
      expect(router.currentRoute.value.name).toBe('dev')
      await vi.waitFor(() => {
        expect(wrapper.find('[data-testid="dev-view"]').exists()).toBe(true)
      })
    } finally {
      wrapper.unmount()
    }
  })

  it('routes DetailPane Edit to actions settings', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/')
    await router.isReady()
    const wrapper = mount(App, {
      global: {
        plugins: [router],
        stubs: {
          DetailPane: { template: '<button data-testid="detail-edit" @click="$emit(\'edit\')" />', emits: ['edit'] },
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-testid="detail-edit"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value).toMatchObject({ name: 'application-settings', params: { section: 'actions' } })
    expect(wrapper.find('[data-testid="actions-settings"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders the actions settings deep-link and preserves it through back/forward history', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/settings/actions')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('actions')
    expect(wrapper.find('[data-testid="actions-settings"]').exists()).toBe(true)

    await router.push('/settings/integrations')
    await flushPromises()
    router.back()
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('actions')
    expect(wrapper.find('[data-testid="actions-settings"]').exists()).toBe(true)
    router.forward()
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('integrations')
    wrapper.unmount()
  })

  it('opens the feed a menu bar heading names', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    const open = mocks.On.mock.calls.find(([event]) => event === 'menubar:open')?.[1] as
      ((ev: { data: unknown }) => void) | undefined

    open?.({ data: { profileId: 'personal', feedId: 'personal/desktop', itemId: 0, settings: false } })
    await flushPromises()

    expect(router.currentRoute.value.query.feed).toBe('personal/desktop')
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="personal/desktop"]').classes()).toContain(
      'sidebar-entry-selected',
    )
    wrapper.unmount()
  })

  it('records feed and unread navigation in back/forward history', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="sidebar-feed"][data-id="personal/desktop"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.feed).toBe('personal/desktop')
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="personal/desktop"]').classes()).toContain(
      'sidebar-entry-selected',
    )

    await wrapper.find('[data-testid="filter-unread"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ feed: 'personal/desktop', unread: '1' })

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ feed: 'personal/desktop' })
    expect(wrapper.find('[data-testid="filter-all"]').attributes('aria-pressed')).toBe('true')

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})
    // A bare feed route selects the profile default: the last-selected feed.
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="personal/desktop"]').classes()).toContain(
      'sidebar-entry-selected',
    )

    router.forward()
    await flushPromises()
    expect(router.currentRoute.value.query.feed).toBe('personal/desktop')

    wrapper.unmount()
  })

  it('routes to trash and loads it via the dedicated trash query', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.get('[data-testid="sidebar-trash"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ view: 'trash' })
    expect(mocks.ListTrash).toHaveBeenLastCalledWith('personal', 500)
    expect(wrapper.get('[data-testid="sidebar-trash"]').classes()).toContain('footer-entry-selected')
    wrapper.unmount()
  })

  it('clears stale observed activity while the selected item timeline loads or fails', async () => {
    const items = [
      {
        id: 1,
        profileId: 'personal',
        sourceKind: 'github',
        sourceScope: '',
        externalId: 'pr-1',
        title: 'First',
        url: '',
        payload: { kind: 'PR', repo: 'acme/app', num: 1, author: 'hay' },
        revision: 1,
        unread: true,
        lifecycle: 'active',
        firstSeenAt: 1,
        lastEventAt: 2,
      },
      {
        id: 2,
        profileId: 'personal',
        sourceKind: 'github',
        sourceScope: '',
        externalId: 'pr-2',
        title: 'Second',
        url: '',
        payload: { kind: 'PR', repo: 'acme/app', num: 2, author: 'hay' },
        revision: 1,
        unread: true,
        lifecycle: 'active',
        firstSeenAt: 1,
        lastEventAt: 2,
      },
    ]
    let rejectSecond!: (error: Error) => void
    const secondEvents = new Promise<never>((_, reject) => {
      rejectSecond = reject
    })
    mocks.ListByFeed.mockResolvedValue(items)
    mocks.Events.mockImplementation((id: number) =>
      id === 1
        ? Promise.resolve([
            {
              id: 1,
              itemId: 1,
              kind: 'observed',
              transition: 'none',
              attention: 'activity',
              summary: 'first event',
              detail: {},
              createdAt: 1,
            },
          ])
        : secondEvents,
    )
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const wrapper = await mountApp()
    expect(wrapper.get('[data-testid="observed-activity"]').text()).toContain('first event')

    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="observed-activity"]').exists()).toBe(false)

    rejectSecond(new Error('events unavailable'))
    await flushPromises()
    expect(wrapper.find('[data-testid="observed-activity"]').exists()).toBe(false)
    expect(warn).toHaveBeenCalledWith('Unable to load inbox item events', expect.any(Error))
    wrapper.unmount()
  })

  it('guards native back navigation when the flow has un-deployed changes', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')
    router.back()
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('flows')
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-discard"]')?.click()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('feed')
    expect(session.dirty.value).toBe(false)

    wrapper.unmount()
  })

  it('deletes the active profile from profile settings, then lands on the replacement', async () => {
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="sidebar-delete-profile"]').exists()).toBe(false)
    await wrapper.find('[data-testid="sidebar-open-settings"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="profile-settings-danger"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="profile-settings-delete"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-testid="delete-profile-modal"]')).not.toBeNull()

    mocks.ListFlows.mockResolvedValue([{ id: 'default', name: 'Default', enabled: true, valid: true, nodes: 0 }])
    document.querySelector<HTMLButtonElement>('[data-testid="delete-profile-confirm"]')?.click()
    await flushPromises()

    expect(mocks.DeleteFlow).toHaveBeenCalledWith('personal')
    expect(document.querySelector('[data-testid="delete-profile-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Default')

    wrapper.unmount()
  })

  it('binds the active profile draft even with the flows canvas closed (hc-8ft4yhm6)', async () => {
    const wrapper = await mountApp()

    // GetLayout/NodeRuns are only ever called from usePipelineEditor's
    // selectFlow — never from useFeedState — so seeing them here proves
    // the app-wide session selected and loaded the active profile draft even
    // though the flows canvas was never opened.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(mocks.GetLayout).toHaveBeenCalledWith('personal')
    expect(mocks.NodeRuns).toHaveBeenCalledWith('personal', 100)

    wrapper.unmount()
  })

  it('reconciles deployed runtimes when flows:updated arrives with the canvas closed', async () => {
    const wrapper = await mountApp()
    const getFlowCalls = mocks.GetFlow.mock.calls.length
    const handler = mocks.On.mock.calls.find(([event]) => event === 'flows:updated')?.[1] as (() => void) | undefined

    expect(handler).toBeDefined()
    handler?.()
    await vi.waitFor(() => expect(mocks.GetFlow.mock.calls.length).toBeGreaterThan(getFlowCalls))

    wrapper.unmount()
  })

  it('the titlebar error chip deep-links to the first failing node, even with the canvas closed', async () => {
    mocks.NodeRuns.mockResolvedValue([
      {
        flowId: 'personal',
        nodeId: 'src',
        ok: false,
        inCount: 0,
        outCount: 0,
        dropCount: 0,
        err: 'boom',
        durMs: 1,
        endedAt: 0,
      },
    ])

    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    const chip = wrapper.find('[data-testid="titlebar-error-chip"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('1 error')

    await chip.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)
    const session = useFlowsSession()
    expect(session.flowFocusNodeId.value).toBe('src')

    wrapper.unmount()
  })

  // ── Un-deployed changes guard (hc-sx4k3c7k) ──────────────────────────────

  it('shows the un-deployed changes badge in the sidebar once the flow is dirty', async () => {
    const wrapper = await mountApp()
    expect(wrapper.find('[data-testid="undeployed-badge"]').exists()).toBe(false)

    useFlowsSession().addNode('feed')
    await flushPromises()

    expect(wrapper.find('[data-testid="undeployed-badge"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('exiting the canvas via the profile rail while dirty prompts a confirm instead of leaving immediately; Cancel stays in the canvas', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)

    const session = useFlowsSession()
    session.addNode('feed')
    expect(session.dirty.value).toBe(true)

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    // Still in the canvas — the exit was deferred behind the confirm modal.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-cancel"]')?.click()
    await flushPromises()

    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true) // cancel aborted the exit
    expect(session.dirty.value).toBe(true) // draft untouched

    wrapper.unmount()
  })

  it('exiting the canvas via the profile rail while dirty: Deploy saves the draft then returns to the feed view', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-deploy"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).toHaveBeenCalled()
    expect(mocks.SaveLayout).toHaveBeenCalled()
    expect(session.dirty.value).toBe(false)
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('exiting the canvas via the profile rail while dirty: Discard drops the draft (reloads from disk) then returns to the feed view', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')
    const getFlowCallsBefore = mocks.GetFlow.mock.calls.length

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-discard"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).not.toHaveBeenCalled()
    expect(mocks.GetFlow.mock.calls.length).toBeGreaterThan(getFlowCallsBefore) // discard reloaded from disk
    expect(session.dirty.value).toBe(false)
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('switching profiles from the rail while dirty prompts a confirm instead of switching immediately; Cancel stays on the current profile', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    const session = useFlowsSession()
    session.addNode('feed') // dirties the active ("personal") flow's draft
    expect(session.dirty.value).toBe(true)

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    // Still on the personal profile — the switch was deferred behind the confirm modal.
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Personal')
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-cancel"]')?.click()
    await flushPromises()

    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Personal') // cancel aborted the switch
    expect(session.dirty.value).toBe(true) // draft untouched
    expect(mocks.GetLayout).not.toHaveBeenCalledWith('work')

    wrapper.unmount()
  })

  it('switching profiles from the rail while dirty: Deploy saves the draft then switches profiles', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    const session = useFlowsSession()
    session.addNode('feed')

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-deploy"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).toHaveBeenCalled()
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Work')
    expect(mocks.GetLayout).toHaveBeenCalledWith('work')

    wrapper.unmount()
  })

  it('switching profiles from the rail while dirty: Discard drops the draft then switches profiles', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    const session = useFlowsSession()
    session.addNode('feed')

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-discard"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Work')
    expect(mocks.GetLayout).toHaveBeenCalledWith('work')

    wrapper.unmount()
  })

  it('switching profiles from the rail is instant (no confirm) when the flow is not dirty', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    expect(useFlowsSession().dirty.value).toBe(false)

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Work')

    wrapper.unmount()
  })

  it('refreshes the feed on "inbox:updated" — the engine commits before it announces', async () => {
    const wrapper = await mountApp()

    mocks.FeedCounts.mockClear()
    const inboxHandler = mocks.On.mock.calls.find(([event]) => event === 'inbox:updated')?.[1] as
      (() => void) | undefined
    expect(inboxHandler).toBeDefined()

    inboxHandler?.()
    await vi.waitFor(() => expect(mocks.FeedCounts).toHaveBeenCalled())

    wrapper.unmount()
  })

  it('does not re-read on "log:appended" — a log row may route nowhere, and the engine has not committed yet', async () => {
    const wrapper = await mountApp()

    const logHandler = mocks.On.mock.calls.find(([event]) => event === 'log:appended')?.[1]
    expect(logHandler).toBeUndefined()

    wrapper.unmount()
  })

  it('swaps the whole hub for terminal mode and back from the title-bar toggle', async () => {
    mocks.TerminalAvailable.mockResolvedValue({ available: false, reason: 'tmux is not installed.' })
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    // Terminal mode is async-imported, so it lands a tick after the toggle.
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
    await flushPromises()

    // The mode is a route, so the toggle is ordinary navigation.
    expect(router.currentRoute.value.name).toBe('terminal')
    // The toggle is never gated on availability; the reason shows up inside.
    expect(wrapper.get('[data-testid="terminal-unavailable-reason"]').text()).toBe('tmux is not installed.')
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(false)
    // Terminal owns a left panel too, so its toggle stays live; the feed-only
    // preview remains unavailable.
    expect(wrapper.get('[data-testid="titlebar-toggle-sidebar"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="titlebar-toggle-preview"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="titlebar-mode-hub"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('feed')
    expect(terminalOnScreen(wrapper)).toBe(false)
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(true)
    // Hidden, not unmounted: the pool behind it holds live tmux clients and
    // xterm screens, and re-entry must not pay to build them again.
    expect(wrapper.find('[data-testid="terminal-mode"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it.each(['metaKey', 'ctrlKey'] as const)(
    'toggles the active sidebar from the title bar, keyboard, and palette (%s)',
    async (modifier) => {
      Object.defineProperty(navigator, 'userAgent', {
        configurable: true,
        value: modifier === 'metaKey' ? 'Macintosh' : 'Windows',
      })
      localStorage.setItem('hive.panel.sidebar.collapsed', 'false')
      localStorage.setItem('hive.panel.terminal.sidebar.collapsed', 'true')
      mocks.TerminalAvailable.mockResolvedValue({ available: true, reason: '' })
      const { wrapper } = await mountAppWithRouter()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'b', [modifier]: true }))
      await flushPromises()
      expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(false)
      expect(localStorage.getItem('hive.panel.sidebar.collapsed')).toBe('true')

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'b', [modifier]: true }))
      await flushPromises()
      expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

      await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
      await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
      await flushPromises()

      const toggle = wrapper.get('[data-testid="titlebar-toggle-sidebar"]')
      expect(toggle.attributes('disabled')).toBeUndefined()
      expect(toggle.attributes('aria-label')).toBe('Show sidebar')
      expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(false)

      await toggle.trigger('click')
      await flushPromises()
      expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(true)
      expect(localStorage.getItem('hive.panel.terminal.sidebar.collapsed')).toBe('false')

      await toggle.trigger('click')
      await flushPromises()
      expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(false)
      expect(localStorage.getItem('hive.panel.terminal.sidebar.collapsed')).toBe('true')

      const pane = focusedPane()
      if (modifier === 'metaKey') {
        const prefix = new KeyboardEvent('keydown', { key: 'b', ctrlKey: true, bubbles: true, cancelable: true })
        pane.dispatchEvent(prefix)
        await flushPromises()
        expect(prefix.defaultPrevented).toBe(false)
        expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(false)
      }
      const shortcut = new KeyboardEvent('keydown', { key: 'b', [modifier]: true, bubbles: true, cancelable: true })
      pane.dispatchEvent(shortcut)
      await flushPromises()
      expect(shortcut.defaultPrevented).toBe(true)
      expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(true)

      const { results } = useCommandPalette()
      void results.value.find((command) => command.id === 'terminal.toggle-sidebar')!.run()
      await flushPromises()
      expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(false)

      const kb = useKeybindings()
      kb.removeBinding('terminal.toggle-sidebar', 'mod+b')
      kb.addBinding('terminal.toggle-sidebar', 'mod+j')
      pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'b', [modifier]: true, bubbles: true }))
      await flushPromises()
      expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(false)
      pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', [modifier]: true, bubbles: true }))
      await flushPromises()
      expect(wrapper.find('[data-testid="terminal-session-sidebar"]').exists()).toBe(true)
      pane.remove()

      await wrapper.get('[data-testid="titlebar-mode-hub"]').trigger('click')
      await flushPromises()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', [modifier]: true }))
      await flushPromises()
      expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(false)
      expect(localStorage.getItem('hive.panel.sidebar.collapsed')).toBe('true')

      wrapper.unmount()
    },
  )

  // #432: the canvas is the right-hand pane in Chats, so the title bar's
  // right-panel toggle drives it — the same slot the detail preview uses in
  // Inbox. It used to carry its own button in the pane status bar.
  it('drives the Chats canvas from the title bar right-panel toggle', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.get('[data-testid="titlebar-mode-agents"]').trigger('click')
    await vi.waitFor(() => expect(agentsOnScreen(wrapper)).toBe(true))
    await flushPromises()

    // No chat open: the canvas has nothing to show beside, so the slot is off.
    expect(wrapper.get('[data-testid="titlebar-toggle-preview"]').attributes('disabled')).toBe('')

    await router.replace({ name: 'agents', params: { workspace: 'web-app' }, query: { chat: '7' } })
    await flushPromises()

    const toggle = wrapper.get('[data-testid="titlebar-toggle-preview"]')
    expect(toggle.attributes('disabled')).toBeUndefined()
    expect(toggle.attributes('aria-label')).toBe('Show preview')

    await toggle.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBe('1')
    expect(wrapper.get('[data-testid="titlebar-toggle-preview"]').attributes('aria-label')).toBe('Hide preview')

    await wrapper.get('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()

    // The Inbox preview is a different pane on the same slot and is untouched.
    expect(localStorage.getItem('hive.panel.detailpane.collapsed')).not.toBe('true')

    wrapper.unmount()
  })

  it('toggles the Chats sidebar from the title bar and global shortcut, and restores it on the focus chord', async () => {
    localStorage.setItem('hive.panel.sidebar.collapsed', 'false')
    localStorage.setItem('hive.panel.terminal.sidebar.collapsed', 'false')
    localStorage.setItem('hive.panel.agents.sidebar.collapsed', 'true')
    const { wrapper } = await mountAppWithRouter()

    await wrapper.get('[data-testid="titlebar-mode-agents"]').trigger('click')
    await vi.waitFor(() => expect(agentsOnScreen(wrapper)).toBe(true))
    await flushPromises()

    const toggle = wrapper.get('[data-testid="titlebar-toggle-sidebar"]')
    expect(toggle.attributes('disabled')).toBeUndefined()
    expect(toggle.attributes('aria-label')).toBe('Show sidebar')

    const pane = focusedPane()
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'b', metaKey: true, bubbles: true }))
    await flushPromises()
    expect(localStorage.getItem('hive.panel.agents.sidebar.collapsed')).toBe('false')
    pane.remove()

    await toggle.trigger('click')
    await flushPromises()
    expect(localStorage.getItem('hive.panel.agents.sidebar.collapsed')).toBe('true')
    // The other two modes keep their own panel out of it.
    expect(localStorage.getItem('hive.panel.sidebar.collapsed')).toBe('false')
    expect(localStorage.getItem('hive.panel.terminal.sidebar.collapsed')).toBe('false')

    // agents.focus-sidebar asks to work in the list, so a hidden one comes back.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(localStorage.getItem('hive.panel.agents.sidebar.collapsed')).toBe('false')

    wrapper.unmount()
  })

  it('keeps the title-bar navigation live inside terminal mode', async () => {
    mocks.TerminalAvailable.mockResolvedValue({ available: false, reason: 'tmux is not installed.' })
    const { wrapper, router } = await mountAppWithRouter()

    // Start somewhere other than the feed so "back to the hub" is observable
    // as "back to where the hub was", not "back to the default feed".
    await router.push({ name: 'application-settings', params: { section: 'integrations' } })
    await flushPromises()

    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))

    // The Inbox toggle lands on the page that mode was left on, not the feed.
    await wrapper.get('[data-testid="titlebar-mode-hub"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')
    expect(router.currentRoute.value.params.section).toBe('integrations')

    // Activity is reachable from inside terminal mode without toggling first.
    // It is an overlay (#441), so it opens over the terminal rather than
    // navigating away from it — the mode stays mounted and on screen.
    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
    await wrapper.get('[data-testid="titlebar-activity"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-testid="activity-overlay"]')).not.toBeNull()
    expect(router.currentRoute.value.name).toBe('terminal')
    expect(terminalOnScreen(wrapper)).toBe(true)

    // Closing it leaves the terminal exactly where it was, with no history step.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[data-testid="activity-overlay"]')).toBeNull()
    expect(router.currentRoute.value.name).toBe('terminal')
    expect(terminalOnScreen(wrapper)).toBe(true)

    wrapper.unmount()
  })

  // #441: Activity used to be its own full-frame route. It is an overlay now,
  // the same shape as Tasks, so opening it never changes where you are.
  it('opens the activity overlay over the current route, and toggles it back off from the icon', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push({ name: 'application-settings', params: { section: 'integrations' } })
    await flushPromises()

    await wrapper.get('[data-testid="titlebar-activity"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-testid="activity-overlay"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="activity-view"]')).not.toBeNull()
    expect(router.currentRoute.value.name).toBe('application-settings')
    expect(router.currentRoute.value.params.section).toBe('integrations')

    await wrapper.get('[data-testid="titlebar-activity"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-testid="activity-overlay"]')).toBeNull()
    expect(router.currentRoute.value.name).toBe('application-settings')

    wrapper.unmount()
  })

  it('resolves an Activity item link and reveals the matching inbox row', async () => {
    const linkedItem = {
      ...inboxItems()[0],
      externalId: 'acme/app#1',
      sourceScope: 'acme/app',
      url: 'https://github.com/acme/app/pull/1',
    }
    mocks.ActivityList.mockResolvedValue([
      {
        id: 10,
        createdAt: Date.now(),
        category: 'auto_action',
        severity: 'auto',
        title: 'Auto-action · My PR approved',
        metadata: {
          'link.item.profileId': 'personal',
          'link.item.sourceKind': 'github',
          'link.item.sourceScope': 'acme/app',
          'link.item.externalId': 'acme/app#1',
        },
      },
    ])
    mocks.FindItems.mockResolvedValue([linkedItem])
    mocks.Feed.mockResolvedValue('personal/desktop')
    mocks.ListByFeed.mockResolvedValue([linkedItem])
    await useActivity().reload()

    const { wrapper, router } = await mountAppWithRouter()
    await wrapper.get('[data-testid="titlebar-activity"]').trigger('click')
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="activity-open-item-10"]')?.click()
    await flushPromises()

    expect(mocks.FindItems).toHaveBeenCalledWith('personal', 'acme/app#1')
    expect(mocks.Feed).toHaveBeenCalledWith('personal', linkedItem.id)
    expect(router.currentRoute.value.name).toBe('feed')
    expect(router.currentRoute.value.query).toEqual({ feed: 'personal/desktop', item: String(linkedItem.id) })
    expect(document.querySelector('[data-testid="activity-overlay"]')).toBeNull()

    wrapper.unmount()
  })

  // ViewHeader ships no close button, so the overlay supplies its own — the
  // title bar's back arrow no longer applies to a surface that is not a page.
  it('closes the activity overlay from its own X and from the backdrop', async () => {
    const { wrapper } = await mountAppWithRouter()

    await wrapper.get('[data-testid="titlebar-activity"]').trigger('click')
    await flushPromises()
    const close = document.querySelector('[data-testid="activity-close"]') as HTMLElement
    expect(close).not.toBeNull()
    close.click()
    await flushPromises()
    expect(document.querySelector('[data-testid="activity-overlay"]')).toBeNull()

    await wrapper.get('[data-testid="titlebar-activity"]').trigger('click')
    await flushPromises()
    const backdrop = document.querySelector('[data-testid="activity-overlay-backdrop"]') as HTMLElement
    backdrop.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="activity-overlay"]')).toBeNull()

    wrapper.unmount()
  })

  it('opens the tasks overlay over the current route without navigating, and closes it on Escape', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('feed')
    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="tasks-view"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).toBeNull()
    expect(router.currentRoute.value.name).toBe('feed')

    wrapper.unmount()
  })

  it('replaces the hand-written tasks palette row with the bindable command, carrying its live shortcut', async () => {
    const wrapper = await mountApp()
    const { results, query } = useCommandPalette()
    query.value = ''

    const matches = results.value.filter((cmd) => cmd.id === 'tasks.toggle' || cmd.id === 'view:tasks')
    expect(matches).toHaveLength(1)
    expect(matches[0].id).toBe('tasks.toggle')
    expect(matches[0].title).toBe('Toggle Tasks')
    expect(matches[0].hint).toBe(formatCombo('mod+shift+t'))

    wrapper.unmount()
  })

  it('toggles the tasks overlay open and closed with mod+shift+T', async () => {
    const { wrapper } = await mountAppWithRouter()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).toBeNull()

    wrapper.unmount()
  })

  it('toggles the canvas overlay over the current route with mod+shift+P, and closes it on Escape', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    const canvasChord = { key: 'p', metaKey: true, shiftKey: true }

    window.dispatchEvent(new KeyboardEvent('keydown', canvasChord))
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('feed')
    expect(document.querySelector('[data-testid="canvas-overlay"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="canvas-page"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', canvasChord))
    await flushPromises()
    expect(document.querySelector('[data-testid="canvas-overlay"]')).toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', canvasChord))
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[data-testid="canvas-overlay"]')).toBeNull()
    expect(router.currentRoute.value.name).toBe('feed')

    wrapper.unmount()
  })

  // No chat is open and the Chats area never mounted: a canvas row is a way
  // to a canvas from anywhere, so it opens the full-page view, not the pane.
  it('lists a canvas as a palette row under its workspace and opens it full page', async () => {
    mocks.AgentsAvailable.mockResolvedValue({ available: true, reason: '' })
    const canvas = vi.fn().mockResolvedValue({
      workspace: 'my-workspace',
      name: 'perf-report',
      title: 'Perf report',
      session: 42,
      createdAt: 1,
      updatedAt: 1,
      blocks: [{ id: 'doc', kind: 'markdown', title: '', body: 'p95 is 96ms', url: '', createdAt: 1, updatedAt: 1 }],
    })
    mocks.agentsClient = {
      workspaces: vi.fn().mockResolvedValue({
        root: '',
        rootProblem: '',
        available: true,
        error: '',
        editor: { command: '', title: '' },
        presets: [],
        workspaces: [{ dir: 'my-workspace', name: 'Travel', command: 'claude', mcps: [], skills: [], schedules: [] }],
      }),
      allSessions: vi.fn().mockResolvedValue([]),
      canvasRepositories: vi.fn().mockResolvedValue([]),
      canvases: vi.fn().mockResolvedValue([
        {
          workspace: 'my-workspace',
          name: 'perf-report',
          title: 'Perf report',
          session: 42,
          hiveSession: '',
          createdAt: 1,
          updatedAt: 1,
          blockCount: 1,
        },
      ]),
      canvas,
    } as unknown as AgentWorkspacesClient
    const { wrapper, router } = await mountAppWithRouter()

    const palette = useCommandPalette()
    palette.toggle()
    await flushPromises()
    palette.query.value = ''
    const row = palette.results.value.find((candidate) => candidate.id === 'canvas:my-workspace:perf-report')
    expect(row?.title).toBe('Perf report')
    expect(row?.group).toBe('Travel')
    expect(row?.kind).toBe('canvas')

    await palette.run(row!)
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('feed')
    expect(canvas).toHaveBeenCalledWith('my-workspace', 'perf-report')
    expect(document.querySelector('[data-testid="canvas-page-block-doc"]')?.textContent).toContain('p95 is 96ms')

    wrapper.unmount()
  })

  // A hive session's canvases belong to its repository, so the title bar's
  // right toggle and the full-page view both work from Code with no chat.
  it('opens the canvas beside a Code session from the title bar, and the full page on its repository', async () => {
    mocks.AgentsAvailable.mockResolvedValue({ available: true, reason: '' })
    const canvases = vi.fn().mockResolvedValue([])
    mocks.agentsClient = {
      workspaces: vi.fn().mockResolvedValue({
        root: '',
        rootProblem: '',
        available: true,
        error: '',
        editor: { command: '', title: '' },
        presets: [],
        workspaces: [],
      }),
      allSessions: vi.fn().mockResolvedValue([]),
      canvasRepositories: vi.fn().mockResolvedValue([]),
      canvases,
    } as unknown as AgentWorkspacesClient
    vi.mocked(ListSessions).mockResolvedValue([
      {
        id: 's1',
        name: 'fix the parser',
        slug: 'hive-fix-parser',
        repo: 'git@github.com:hay-kot/hive.git',
        state: 'active',
        canvasOwner: 'hay-kot/hive',
      },
    ])
    const { wrapper, router } = await mountAppWithRouter()

    // The scratch terminal has no checkout, so it has no canvas to toggle.
    await router.push('/terminal/Scratch')
    await flushPromises()
    expect(wrapper.get('[data-testid="titlebar-toggle-preview"]').attributes('disabled')).toBeDefined()

    await router.push('/terminal/hive-fix-parser')
    await flushPromises()
    await vi.waitFor(() =>
      expect(wrapper.get('[data-testid="titlebar-toggle-preview"]').attributes('disabled')).toBeUndefined(),
    )
    await wrapper.get('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBe('1')
    expect(router.currentRoute.value.params.slug).toBe('hive-fix-parser')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="canvas-overlay"]')).not.toBeNull()
    expect(canvases).toHaveBeenCalledWith('hay-kot/hive')

    vi.mocked(ListSessions).mockResolvedValue([])
    wrapper.unmount()
  })

  it('suppresses the tasks toggle while a different overlay is open, and while a confirm dialog is stacked inside it', async () => {
    const taskItem = {
      id: 't1',
      repoKey: 'acme/site',
      epicId: '',
      parentId: '',
      sessionId: '',
      title: 'Task t1',
      type: 'task',
      status: 'open',
      blocked: false,
      depth: 0,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    mocks.ListTasks.mockResolvedValue([taskItem])
    mocks.TaskDetail.mockResolvedValue({ ...taskItem, desc: '', blockers: [], comments: [] })
    const { wrapper } = await mountAppWithRouter()
    const { openBundleDialog: openReport, close: closeReport } = useReportDialog()

    // A different modal swallows the toggle like any other command.
    openReport()
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).toBeNull()
    closeReport()
    await flushPromises()

    // Opens normally once nothing else is up.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()

    // A confirm dialog stacked inside the overlay (its own BaseModal) keeps
    // the toggle from also closing the overlay underneath it.
    document.querySelector<HTMLButtonElement>('[data-testid="task-delete"]')!.click()
    await flushPromises()
    expect(document.querySelector('[data-testid="task-delete-confirm"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="task-delete-confirm"]')).not.toBeNull()

    wrapper.unmount()
  })

  // TerminalMode reports the attached session's resolved owner/repo
  // continuously (not only on a click of its own status-bar button), so
  // App.vue can scope Tasks to it from any entry point — the keybinding and
  // the palette included, both of which funnel through the same openTasks().
  it('scopes tasks to the terminal session repo when the keybinding opens it in terminal context', async () => {
    const { wrapper } = await mountAppWithRouter()
    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
    await wrapper.findComponent(TerminalMode).vm.$emit('session-repo-key', 'acme/site')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()

    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    expect(useTasks().repoKey.value).toBe('acme/site')

    wrapper.unmount()
  })

  it('keeps a user-picked scope when the keybinding closes tasks from terminal context', async () => {
    const { wrapper } = await mountAppWithRouter()
    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
    await wrapper.findComponent(TerminalMode).vm.$emit('session-repo-key', 'acme/site')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    expect(useTasks().repoKey.value).toBe('acme/site')

    // The user re-scopes while the overlay is open; only an *opening* toggle
    // may re-resolve the scope, so closing must not clobber the choice.
    useTasks().setRepoKey('acme/other')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="tasks-overlay"]')).toBeNull()
    expect(useTasks().repoKey.value).toBe('acme/other')

    wrapper.unmount()
  })

  it('leaves the persisted scope alone when the keybinding opens tasks from the hub', async () => {
    const { wrapper } = await mountAppWithRouter()
    useTasks().setRepoKey('acme/existing')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true, shiftKey: true }))
    await flushPromises()

    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    expect(useTasks().repoKey.value).toBe('acme/existing')

    wrapper.unmount()
  })

  it('scopes tasks to the repo carried by the terminal status bar’s own open-tasks click', async () => {
    const { wrapper } = await mountAppWithRouter()
    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
    const terminal = wrapper.findComponent(TerminalMode)
    await terminal.vm.$emit('session-repo-key', 'acme/site')

    await terminal.vm.$emit('open-tasks')
    await flushPromises()

    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    expect(useTasks().repoKey.value).toBe('acme/site')

    wrapper.unmount()
  })

  it('opens tasks at the persisted scope when the terminal session has no resolved repo', async () => {
    const { wrapper } = await mountAppWithRouter()
    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
    useTasks().setRepoKey('acme/existing')
    const terminal = wrapper.findComponent(TerminalMode)
    await terminal.vm.$emit('session-repo-key', '')

    await terminal.vm.$emit('open-tasks')
    await flushPromises()

    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    expect(useTasks().repoKey.value).toBe('acme/existing')

    wrapper.unmount()
  })

  it('re-enters terminal mode on the session it was left attached to', async () => {
    mocks.TerminalAvailable.mockResolvedValue({ available: true, reason: '' })
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await vi.waitFor(() => expect(terminalOnScreen(wrapper)).toBe(true))
    await router.replace({ name: 'terminal', params: { slug: 'api-fix' }, query: { window: '@3' } })
    await flushPromises()

    await wrapper.get('[data-testid="titlebar-mode-hub"]').trigger('click')
    await flushPromises()
    expect(terminalOnScreen(wrapper)).toBe(false)

    // Straight back to the attached session and window, not through the bare
    // picker route — that pass detached the pool entry and blanked the pane.
    await wrapper.get('[data-testid="titlebar-mode-terminal"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('terminal')
    expect(router.currentRoute.value.params.slug).toBe('api-fix')
    expect(router.currentRoute.value.query.window).toBe('@3')

    wrapper.unmount()
  })

  it('swaps the whole hub for the Agents area and back from the title-bar toggle, hiding rather than unmounting it', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.get('[data-testid="titlebar-mode-agents"]').trigger('click')
    // AgentsMode is async-imported, so it lands a tick after the toggle.
    await vi.waitFor(() => expect(agentsOnScreen(wrapper)).toBe(true))
    await flushPromises()

    // The mode is a route, so the toggle is ordinary navigation.
    expect(router.currentRoute.value.name).toBe('agents')
    // The toggle is never gated on availability; the reason shows up inside.
    expect(wrapper.get('[data-testid="agents-unavailable-reason"]').text()).toBe('no ptyterm on this build.')
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(false)

    await wrapper.get('[data-testid="titlebar-mode-hub"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('feed')
    expect(agentsOnScreen(wrapper)).toBe(false)
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(true)
    // Hidden, not unmounted: leaving the area must not end a live session's
    // pane (ADR terminal-mode-is-hidden-not-unmounted).
    expect(wrapper.find('[data-testid="agents-mode"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('renders all three mode segments once a profile exists', async () => {
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="titlebar-mode-hub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="titlebar-mode-terminal"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="titlebar-mode-agents"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('lets a focused terminal keep every key a pane can use', async () => {
    const wrapper = await mountApp()
    const { open: paletteOpen } = useCommandPalette()

    const pane = document.createElement('div')
    pane.setAttribute('data-terminal-input-scope', '')
    document.body.append(pane)

    // Ctrl+K is readline's kill-to-end-of-line, and `mod+k` cannot tell it from
    // ⌘K — so the pane keeps it even though it resolves to the palette.
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))
    // A bare navigation key is the pane's outright.
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', bubbles: true }))
    await flushPromises()
    expect(paletteOpen.value).toBe(false)

    pane.remove()
    wrapper.unmount()
  })

  // The palette is how you get back out of a pane, so it is the exception to
  // the rule above — on the modifiers a terminal never wants.
  it('opens the palette over a focused terminal on Cmd, and on Ctrl+Shift', async () => {
    const wrapper = await mountApp()
    const { open: paletteOpen } = useCommandPalette()

    const pane = document.createElement('div')
    pane.setAttribute('data-terminal-input-scope', '')
    document.body.append(pane)

    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true }))
    await flushPromises()
    expect(paletteOpen.value).toBe(true)

    paletteOpen.value = false
    await flushPromises()
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, shiftKey: true, bubbles: true }))
    await flushPromises()
    expect(paletteOpen.value).toBe(true)

    paletteOpen.value = false
    pane.remove()
    wrapper.unmount()
  })

  // The other way out of a pane. Only this half of the focus pair pierces: the
  // chord that moves focus *into* a pane is unreachable from inside one.
  it('reaches the session tree from inside a focused terminal', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    const { focusTree } = stubTerminalTree()
    const pane = focusedPane()

    const event = new KeyboardEvent('keydown', { key: 'ArrowLeft', metaKey: true, bubbles: true, cancelable: true })
    pane.dispatchEvent(event)
    await flushPromises()

    expect(focusTree).toHaveBeenCalled()
    // Swallowed here so the webview cannot also read ⌘← as browser Back.
    expect(event.defaultPrevented).toBe(true)

    setTerminalTreeHandles(null)
    pane.remove()
    wrapper.unmount()
  })

  // Same reason: the window you are jumping away from is holding the keyboard.
  it('jumps to a numbered window from inside a focused terminal', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    const { selectWindow } = stubTerminalTree()
    const pane = focusedPane()

    const event = new KeyboardEvent('keydown', { key: '3', metaKey: true, bubbles: true, cancelable: true })
    pane.dispatchEvent(event)
    await flushPromises()

    expect(selectWindow).toHaveBeenCalledWith(3)
    expect(event.defaultPrevented).toBe(true)

    setTerminalTreeHandles(null)
    pane.remove()
    wrapper.unmount()
  })

  // A bare key, so it belongs to whatever holds focus: the tree and the rows
  // answer it, and a focused pane keeps `/` as the character it is.
  it('focuses the session filter on / from the tree, and never from inside a pane', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    const { focusFilter } = stubTerminalTree()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: '/' }))
    await flushPromises()
    expect(focusFilter).toHaveBeenCalled()

    vi.mocked(focusFilter).mockClear()
    const pane = focusedPane()
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true }))
    await flushPromises()
    expect(focusFilter).not.toHaveBeenCalled()

    setTerminalTreeHandles(null)
    pane.remove()
    wrapper.unmount()
  })

  it('leaves the terminal chords alone outside terminal mode', async () => {
    const wrapper = await mountApp()
    const { focusTree, selectWindow, newWindow } = stubTerminalTree()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', metaKey: true }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '2', metaKey: true }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true }))
    await flushPromises()

    expect(focusTree).not.toHaveBeenCalled()
    expect(selectWindow).not.toHaveBeenCalled()
    expect(newWindow).not.toHaveBeenCalled()

    setTerminalTreeHandles(null)
    wrapper.unmount()
  })

  // The tab chords are dispatched from wherever focus is: the tree answers them
  // as an ordinary terminal-context command.
  it('runs the window lifecycle chords from the session tree', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    const { newWindow, closeWindow, stepWindow } = stubTerminalTree()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '}', metaKey: true, shiftKey: true }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '{', metaKey: true, shiftKey: true }))
    await flushPromises()

    expect(newWindow).toHaveBeenCalledTimes(1)
    expect(closeWindow).toHaveBeenCalledTimes(1)
    expect(stepWindow).toHaveBeenNthCalledWith(1, 1)
    expect(stepWindow).toHaveBeenNthCalledWith(2, -1)

    setTerminalTreeHandles(null)
    wrapper.unmount()
  })

  // The pane is where you are when you want another tab, so the chords fire
  // over one — on Command, and on Ctrl+Shift where there is no Command.
  it('runs the window lifecycle chords over a focused terminal, on either platform spelling', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    const { newWindow, closeWindow, stepWindow } = stubTerminalTree()
    const pane = focusedPane()

    const event = new KeyboardEvent('keydown', { key: 't', metaKey: true, bubbles: true, cancelable: true })
    pane.dispatchEvent(event)
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'W', ctrlKey: true, shiftKey: true, bubbles: true }))
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: '}', ctrlKey: true, shiftKey: true, bubbles: true }))
    await flushPromises()

    expect(newWindow).toHaveBeenCalledTimes(1)
    expect(closeWindow).toHaveBeenCalledTimes(1)
    expect(stepWindow).toHaveBeenCalledWith(1)
    expect(event.defaultPrevented).toBe(true)

    setTerminalTreeHandles(null)
    pane.remove()
    wrapper.unmount()
  })

  // The regression this exists for: the chord used to be bound to a launcher,
  // which pierces a pane unconditionally, and rebinding it to the built-in
  // command silently lost that — an alt combo can never qualify as a terminal
  // escape chord, so Tasks was unreachable from inside a session.
  it('opens tasks over a focused terminal on a user-bound alt chord', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    useKeybindings().addBinding('tasks.toggle', 'alt+t')
    const pane = focusedPane()

    const event = new KeyboardEvent('keydown', { key: 't', altKey: true, bubbles: true, cancelable: true })
    pane.dispatchEvent(event)
    await flushPromises()

    expect(document.querySelector('[data-testid="tasks-overlay"]')).not.toBeNull()
    // Swallowed here so the pane does not also write the meta escape to tmux.
    expect(event.defaultPrevented).toBe(true)

    pane.remove()
    wrapper.unmount()
  })

  // The opt-in is per command, which is the whole reason it is a catalog flag
  // rather than a wider escape chord: alt+t stays readline's transpose-words
  // for anything that did not ask for it.
  it('leaves an alt chord with the pane for a command that does not pierce it', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    useKeybindings().addBinding('report.bundle', 'alt+t')
    const pane = focusedPane()

    const event = new KeyboardEvent('keydown', { key: 't', altKey: true, bubbles: true, cancelable: true })
    pane.dispatchEvent(event)
    await flushPromises()

    expect(useReportDialog().open.value).toBe(false)
    expect(event.defaultPrevented).toBe(false)

    // Same chord, same command, from outside a pane: the pane is the only thing
    // withholding it.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', altKey: true }))
    await flushPromises()
    expect(useReportDialog().open.value).toBe(true)

    useReportDialog().close()
    pane.remove()
    wrapper.unmount()
  })

  // The reason the tab chords escape rather than pierce: where `mod` is Ctrl,
  // Ctrl+T is readline's transpose-chars and Ctrl+W its unix-word-rubout.
  it('leaves a focused terminal the bare Ctrl form of the window chords', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await router.push('/terminal/hive-fix-parser')
    await flushPromises()

    const { newWindow, closeWindow } = stubTerminalTree()
    const pane = focusedPane()

    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 't', ctrlKey: true, bubbles: true }))
    pane.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', ctrlKey: true, bubbles: true }))
    await flushPromises()

    expect(newWindow).not.toHaveBeenCalled()
    expect(closeWindow).not.toHaveBeenCalled()

    setTerminalTreeHandles(null)
    pane.remove()
    wrapper.unmount()
  })
})
