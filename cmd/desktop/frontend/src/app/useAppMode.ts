import { computed, ref, watch, type Ref } from 'vue'
import { useStorage } from '@vueuse/core'
import { Window } from '@wailsio/runtime'
import { useDriveRoute } from '../composables/useDriveRoute'
import { docsScopeOf } from '../components/terminal/panelViews'
import { workspaceSpaceKey, type DriveAuthor, type DriveScope } from '../lib/drive'
import { globalTabLocation, isPinnedTabId, tabOfRoute, type AppTabId, type GlobalTabId } from '../lib/appTabs'
import { useAgentSessionsAll } from '../stores/useAgentSessionsAll'
import { useAppTabs } from '../stores/useAppTabs'
import { useDriveReview } from '../stores/useDriveReview'
import { useChatPanelLayouts, usePanelLayouts } from '../stores/usePanelLayouts'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import type { AppNavigation, FeedState } from './useAppNavigation'
import { createLogger } from '../lib/log'
import { AGENTS_SIDEBAR, TERMINAL_SIDEBAR } from '../lib/modeSidebars'

const log = createLogger('app-mode')

export type AppMode = 'hub' | 'terminal' | 'agents' | 'settings' | 'drive' | 'tasks' | 'runs' | 'applet'
export type PinnedMode = 'hub' | 'terminal' | 'agents'

const PINNED_MODES: Record<'inbox' | 'code' | 'chats', PinnedMode> = {
  inbox: 'hub',
  code: 'terminal',
  chats: 'agents',
}

/**
 * Which area owns the frame under the title bar, and that frame's panel chrome.
 * The title bar is a strip of tabs: Inbox (the feed, flows, and profile
 * settings), Code (terminal mode) and Chats (the agents area) are pinned, and
 * Drive, Tasks, Settings and applets are global tabs the user adds and closes.
 * Each tab is a route, so history restores where each was left and a tab
 * opened by a link joins the strip. A relaunch lands on the hub, so none
 * attaches a tmux client or an agent session unprompted.
 */
export function useAppMode(
  feed: FeedState,
  nav: AppNavigation,
  onboardingActive: Ref<boolean>,
  shellLoaded: Ref<boolean>,
) {
  const { route, router } = nav
  const { routeChatId } = useDriveRoute()
  const review = useDriveReview()
  const { recents } = useAgentSessionsAll()
  const { sessions: codeSessions } = useTerminalSessions()
  const codeLayouts = usePanelLayouts()
  const chatLayouts = useChatPanelLayouts()

  const mode = computed<AppMode>(() => {
    if (route.name === 'terminal') return 'terminal'
    if (route.name === 'agents') return 'agents'
    if (route.name === 'application-settings') return 'settings'
    if (route.name === 'drive') return 'drive'
    if (route.name === 'tasks') return 'tasks'
    if (route.name === 'action-runs') return 'runs'
    if (route.name === 'applet') return 'applet'
    return 'hub'
  })
  // Each area is its own positive case rather than "not terminal", so a deep
  // link to one never mounts the hub under it.
  const shown = (area: AppMode) => computed(() => mode.value === area && shellLoaded.value && !onboardingActive.value)
  const terminalActive = shown('terminal')
  const agentsActive = shown('agents')
  const hubActive = shown('hub')
  const settingsActive = shown('settings')
  const driveActive = shown('drive')
  const tasksActive = shown('tasks')
  const runsActive = shown('runs')
  const runsRunId = computed(() => {
    const id = runsActive.value ? Number(route.query.run) : NaN
    return Number.isInteger(id) && id > 0 ? id : null
  })
  const appletActive = shown('applet')
  const openAppletId = computed(() =>
    appletActive.value && typeof route.params.id === 'string' ? route.params.id : '',
  )

  // Mounted on first entry and then only hidden (ADR
  // terminal-mode-is-hidden-not-unmounted): both hold live PTYs and xterm
  // screens bound to their elements, so unmounting paid a full re-attach. The
  // Code view can mount hidden ahead of that (ADR
  // code-and-chats-preload-after-the-hub-is-up).
  const mountedOnce = (active: Ref<boolean>) => {
    const mounted = ref(false)
    watch(
      active,
      (isActive) => {
        if (isActive) mounted.value = true
      },
      { immediate: true },
    )
    return mounted
  }
  const terminalMounted = mountedOnce(terminalActive)
  function prepareTerminal(): void {
    terminalMounted.value = true
  }
  const agentsMounted = mountedOnce(agentsActive)

  // Each tab lands where it was last. Only the agents path survives a reload:
  // ?chat reattaches only a still-live session (ADR the-open-chat-rides-the-route),
  // while a restored /terminal/:slug would attach a tmux control client
  // unconditionally.
  const tabs = useAppTabs()
  let lastHubPath = ''
  let lastTerminalPath = ''
  const lastGlobalPaths = new Map<GlobalTabId, string>()
  const lastAgentsPath = useStorage('hive.mode.agents.path', '')
  if (!lastAgentsPath.value.startsWith('/workspaces')) lastAgentsPath.value = ''

  const activeTab = computed<AppTabId | null>(() => tabOfRoute(route.name, route.params))

  watch(
    () => route.fullPath,
    (path) => {
      const tab = activeTab.value
      if (!tab) return
      tabs.visited(tab)
      if (tab === 'code') lastTerminalPath = path
      else if (tab === 'chats') lastAgentsPath.value = path
      else if (tab === 'inbox') lastHubPath = path
      else {
        // A link, a chord or the palette can open a page that is not in the strip yet.
        tabs.add(tab)
        lastGlobalPaths.set(tab, path)
      }
    },
    { immediate: true },
  )

  function setMode(next: AppMode): void {
    if (next === mode.value) return
    if (next === 'terminal') void router.push(lastTerminalPath || { name: 'terminal' })
    else if (next === 'agents') void router.push(lastAgentsPath.value || { name: 'agents' })
    else void router.push(lastHubPath || { name: 'feed' })
  }

  function activateTab(id: AppTabId): void {
    if (id === activeTab.value) return
    if (isPinnedTabId(id)) {
      setMode(PINNED_MODES[id])
      return
    }
    tabs.add(id)
    void router.push(lastGlobalPaths.get(id) ?? globalTabLocation(id))
  }

  function closeTab(id: GlobalTabId): void {
    const wasActive = activeTab.value === id
    tabs.remove(id)
    lastGlobalPaths.delete(id)
    if (wasActive) activateTab(tabs.previous(id))
  }

  /** A chord's toggle: brings the tab forward, or steps back from it without closing it. */
  function toggleTab(id: GlobalTabId): void {
    if (activeTab.value === id) activateTab(tabs.previous(id))
    else activateTab(id)
  }

  // A run rides the query, so a link opens on it and Back walks the runs opened.
  function openActionRuns(runId?: number): void {
    if (runId) void router.push({ name: 'action-runs', query: { run: String(runId) } })
    else activateTab('runs')
  }

  function closeSettings(): void {
    closeTab('settings')
  }

  // The URL is the attach state, so it is also which session is on screen.
  const onScreenSessionSlug = computed(() =>
    route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
  )

  const feedSidebarCollapsed = useStorage('hive.panel.sidebar.collapsed', false)
  const terminalSidebarCollapsed = useStorage(TERMINAL_SIDEBAR.collapsedKey, false)
  const agentsSidebarCollapsed = useStorage(AGENTS_SIDEBAR.collapsedKey, false)
  const driveSidebarCollapsed = useStorage('hive.panel.drive.sidebar.collapsed', false)
  const previewCollapsed = useStorage('hive.panel.detailpane.collapsed', false)
  // Follows the hub's template fallthrough: the feed renders whenever no other
  // hub page claims the route.
  const feedViewActive = computed(
    () =>
      hubActive.value &&
      !nav.profileSettingsActive.value &&
      !nav.flowsActive.value &&
      !nav.devActive.value &&
      !!feed.activeProfile.value,
  )
  // Bare navigation keys reach the feed only on the feed route itself.
  const feedNavActive = computed(
    () => route.name === 'feed' && !onboardingActive.value && !terminalActive.value && !!feed.activeProfile.value,
  )

  // Null in a view with no left panel (settings, flows), which disables the
  // title-bar toggle.
  const activeSidebarFlag = computed(() => {
    if (terminalActive.value) return terminalSidebarCollapsed
    if (agentsActive.value) return agentsSidebarCollapsed
    if (feedViewActive.value) return feedSidebarCollapsed
    if (driveActive.value) return driveSidebarCollapsed
    return null
  })
  const sidebarCollapsed = computed(() => activeSidebarFlag.value?.value ?? false)
  const canToggleSidebar = computed(() => activeSidebarFlag.value !== null)

  function toggleSidebar(): void {
    const collapsed = activeSidebarFlag.value
    if (collapsed) collapsed.value = !collapsed.value
  }

  // The space the session on screen in Code reads: its repository's for a
  // hive session, its workspace's for a pinned chat. The scratch terminal has
  // none.
  const codeDrive = computed<{ space: string; session: DriveAuthor } | null>(() => {
    const slug = onScreenSessionSlug.value
    if (!terminalActive.value || !slug) return null
    const chat = recents.value.find((row) => row.slug === slug)
    if (chat) return { space: workspaceSpaceKey(chat.workspace), session: chat.id }
    const row = codeSessions.value.find((session) => session.slug === slug)
    return row?.canvasOwner ? { space: row.canvasOwner, session: row.id } : null
  })

  // One control per frame edge: the right toggle names the detail preview in
  // Inbox and the comments on the Drive page. Code and Chats have no right
  // pane: Drive is a session tab in both.
  const previewToggleCollapsed = computed(() =>
    driveActive.value ? review.pageCommentsCollapsed.value : previewCollapsed.value,
  )
  const canTogglePreview = computed(() => feedViewActive.value || (driveActive.value && review.pageReviewed.value))

  function togglePreview(): void {
    if (terminalActive.value) return
    if (driveActive.value) review.togglePageComments()
    else previewCollapsed.value = !previewCollapsed.value
  }

  // What the open chat's or Code session's Drive tab shows, or would show: the
  // full-page view opens on the same document, which the tab carries in its
  // layout. A chat's workspace is its own and need not be the focused one,
  // which is the fallback with no chat open.
  const viewDriveScope = computed<DriveScope | null>(() => {
    if (codeDrive.value) {
      return docsScopeOf(codeLayouts.layoutOf(codeDrive.value.session)) ?? { ...codeDrive.value, path: null }
    }
    if (!agentsActive.value) return null
    const session = routeChatId.value
    if (session) {
      const own = docsScopeOf(chatLayouts.layoutOf(session))
      if (own) return own
    }
    const focused = typeof route.params.workspace === 'string' ? route.params.workspace : ''
    const workspace = recents.value.find((chat) => chat.id === session)?.workspace ?? focused
    return workspace ? { space: workspaceSpaceKey(workspace), path: null, session } : null
  })

  // The full page's scope is its query, so a document is a link and history
  // restores it. A pick inside the page replaces rather than pushes, as the
  // pane's ?doc does, so Back leaves the page instead of walking its picks.
  const str = (value: unknown) => (typeof value === 'string' ? value : '')
  const driveScope = computed<DriveScope>(() => ({
    space: str(route.query.space),
    path: str(route.query.doc) || null,
    session: str(route.query.session) || null,
  }))

  function driveLocation(scope: DriveScope) {
    const query: Record<string, string> = {}
    if (scope.space) query.space = scope.space
    if (scope.path) query.doc = scope.path
    if (scope.session) query.session = scope.session
    return { name: 'drive' as const, query }
  }

  function openDrive(scope: DriveScope): void {
    void router.push(driveLocation(scope))
  }

  function updateDriveScope(scope: DriveScope): void {
    void router.replace(driveLocation(scope))
  }

  function closeDrive(): void {
    closeTab('drive')
  }

  // Opening follows the chat or session on screen; from anywhere else it
  // returns to the document last read.
  function toggleDrive(): void {
    if (mode.value === 'drive') toggleTab('drive')
    else if (viewDriveScope.value) openDrive(viewDriveScope.value)
    else activateTab('drive')
  }

  // The hidden-inset title bar loses the native double-click-to-zoom, so it is
  // re-implemented here. Guarded for the non-Wails test and browser context.
  async function toggleMaximise(): Promise<void> {
    try {
      if (typeof Window?.ToggleMaximise === 'function') await Window.ToggleMaximise()
    } catch (error) {
      log.debug('window maximise is unavailable outside Wails', error)
    }
  }

  // ⌘W closes what is in front: a global tab, or with none the window, which the Go side
  // hides (or quits, with no tray to restore it from) as the macOS Close item did.
  async function closeFront(): Promise<void> {
    const tab = activeTab.value
    if (tab && !isPinnedTabId(tab)) {
      closeTab(tab)
      return
    }
    try {
      if (typeof Window?.Close === 'function') await Window.Close()
    } catch (error) {
      log.debug('window close is unavailable outside Wails', error)
    }
  }

  return {
    mode,
    setMode,
    closeFront,
    activeTab,
    activateTab,
    closeTab,
    toggleTab,
    tasksActive,
    runsActive,
    runsRunId,
    openActionRuns,
    appletActive,
    openAppletId,
    terminalActive,
    agentsActive,
    hubActive,
    settingsActive,
    driveActive,
    driveScope,
    openDrive,
    updateDriveScope,
    closeDrive,
    toggleDrive,
    closeSettings,
    terminalMounted,
    prepareTerminal,
    agentsMounted,
    onScreenSessionSlug,
    feedSidebarCollapsed,
    terminalSidebarCollapsed,
    agentsSidebarCollapsed,
    driveSidebarCollapsed,
    previewCollapsed,
    feedNavActive,
    sidebarCollapsed,
    canToggleSidebar,
    toggleSidebar,
    previewToggleCollapsed,
    canTogglePreview,
    togglePreview,
    codeDrive,
    viewDriveScope,
    toggleMaximise,
  }
}
