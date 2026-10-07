import { computed, ref, watch, type Ref } from 'vue'
import { useStorage } from '@vueuse/core'
import { Window } from '@wailsio/runtime'
import { useAgentCanvasRoute } from '../composables/useAgentCanvasRoute'
import type { CanvasAuthor, CanvasScope } from '../lib/agentCanvas'
import { useAgentSessionsAll } from '../stores/useAgentSessionsAll'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import type { AppNavigation, FeedState } from './useAppNavigation'

export type AppMode = 'hub' | 'terminal' | 'agents' | 'settings'

/**
 * Which area owns the frame under the title bar, and that frame's panel chrome.
 * Inbox (the hub) is the feed, flows, and profile settings; Code is terminal
 * mode; Chats is the agents area. Application settings answer to the whole
 * app, so they are an area of their own rather than a page of the Inbox. Terminal and agents are routes, so the title bar
 * stays live inside them and history restores where each was left. A relaunch
 * lands on the hub, so none attaches a tmux client or an agent session
 * unprompted.
 */
export function useAppMode(
  feed: FeedState,
  nav: AppNavigation,
  onboardingActive: Ref<boolean>,
  shellLoaded: Ref<boolean>,
) {
  const { route, router } = nav
  const { routeChatId, canvasRequested, canvasName, canvasUnseen, isCanvasUnseen, syncCanvasQuery } =
    useAgentCanvasRoute()
  const { recents } = useAgentSessionsAll()
  const { sessions: codeSessions } = useTerminalSessions()

  const mode = computed<AppMode>(() => {
    if (route.name === 'terminal') return 'terminal'
    if (route.name === 'agents') return 'agents'
    if (route.name === 'application-settings') return 'settings'
    return 'hub'
  })
  // Each area is its own positive case rather than "not terminal", so a deep
  // link to one never mounts the hub under it.
  const shown = (area: AppMode) => computed(() => mode.value === area && shellLoaded.value && !onboardingActive.value)
  const terminalActive = shown('terminal')
  const agentsActive = shown('agents')
  const hubActive = shown('hub')
  const settingsActive = shown('settings')

  // Mounted on first entry and then only hidden (ADR
  // terminal-mode-is-hidden-not-unmounted): both hold live PTYs and xterm
  // screens bound to their elements, so unmounting paid a full re-attach.
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
  const agentsMounted = mountedOnce(agentsActive)

  // Each mode's toggle lands where that mode was last. Only the agents path
  // survives a reload: ?chat reattaches only a still-live session (ADR
  // the-open-chat-rides-the-route), while a restored /terminal/:slug would
  // attach a tmux control client unconditionally.
  let lastHubPath = ''
  let lastTerminalPath = ''
  // Where closing settings returns to: the last page of any other area.
  let lastOutsideSettingsPath = ''
  const lastAgentsPath = useStorage('hive.mode.agents.path', '')
  if (!lastAgentsPath.value.startsWith('/workspaces')) lastAgentsPath.value = ''
  watch(
    () => route.fullPath,
    (path) => {
      if (!route.name) return
      if (route.name === 'application-settings') return
      lastOutsideSettingsPath = path
      if (route.name === 'terminal') lastTerminalPath = path
      else if (route.name === 'agents') lastAgentsPath.value = path
      else lastHubPath = path
    },
    { immediate: true },
  )

  function setMode(next: AppMode): void {
    if (next === mode.value) return
    if (next === 'terminal') void router.push(lastTerminalPath || { name: 'terminal' })
    else if (next === 'agents') void router.push(lastAgentsPath.value || { name: 'agents' })
    else void router.push(lastHubPath || { name: 'feed' })
  }

  function closeSettings(): void {
    void router.push(lastOutsideSettingsPath || { name: 'feed' })
  }

  // The URL is the attach state, so it is also which session is on screen.
  const onScreenSessionSlug = computed(() =>
    route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
  )

  const feedSidebarCollapsed = useStorage('hive.panel.sidebar.collapsed', false)
  const terminalSidebarCollapsed = useStorage('hive.panel.terminal.sidebar.collapsed', false)
  const agentsSidebarCollapsed = useStorage('hive.panel.agents.sidebar.collapsed', false)
  const previewCollapsed = useStorage('hive.panel.detailpane.collapsed', false)
  // Follows the hub's template fallthrough: the feed renders whenever no other
  // hub page claims the route.
  const feedViewActive = computed(
    () =>
      !onboardingActive.value &&
      !terminalActive.value &&
      !agentsActive.value &&
      !settingsActive.value &&
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
    return null
  })
  const sidebarCollapsed = computed(() => activeSidebarFlag.value?.value ?? false)
  const canToggleSidebar = computed(() => activeSidebarFlag.value !== null)

  function toggleSidebar(): void {
    const collapsed = activeSidebarFlag.value
    if (collapsed) collapsed.value = !collapsed.value
  }

  // Whose canvases the session on screen in Code reads: its repository's for
  // a hive session, its workspace's for a pinned chat. The scratch terminal
  // has none.
  const codeCanvas = computed<{ workspace: string; session: CanvasAuthor } | null>(() => {
    const slug = onScreenSessionSlug.value
    if (!terminalActive.value || !slug) return null
    const chat = recents.value.find((row) => row.slug === slug)
    if (chat) return { workspace: chat.workspace, session: chat.id }
    const row = codeSessions.value.find((session) => session.slug === slug)
    return row?.canvasOwner ? { workspace: row.canvasOwner, session: row.id } : null
  })

  // One control per frame edge: the right toggle names the detail preview in
  // Inbox and the canvas in Chats and Code (hay-kot/hive-desktop#432).
  const agentsCanvasAvailable = computed(() => agentsActive.value && routeChatId.value !== null)
  const canvasAvailable = computed(() => agentsCanvasAvailable.value || codeCanvas.value !== null)
  const previewToggleCollapsed = computed(() =>
    canvasAvailable.value ? !canvasRequested.value : previewCollapsed.value,
  )
  const canTogglePreview = computed(() => feedViewActive.value || canvasAvailable.value)
  const previewUnseen = computed(() =>
    agentsCanvasAvailable.value ? canvasUnseen.value : isCanvasUnseen(codeCanvas.value?.session ?? null),
  )

  function togglePreview(): void {
    if (canvasAvailable.value) syncCanvasQuery(!canvasRequested.value)
    else previewCollapsed.value = !previewCollapsed.value
  }

  // What the canvas pane beside the open chat or Code session shows, or would
  // show: the full-page view opens on the same canvas. A chat's workspace is
  // its own and need not be the focused one, which is the fallback with no
  // chat open.
  const viewCanvasScope = computed<CanvasScope | null>(() => {
    if (codeCanvas.value) return { ...codeCanvas.value, name: canvasName.value }
    if (!agentsActive.value) return null
    const session = routeChatId.value
    const focused = typeof route.params.workspace === 'string' ? route.params.workspace : ''
    const workspace = recents.value.find((chat) => chat.id === session)?.workspace ?? focused
    return workspace ? { workspace, name: canvasName.value, session } : null
  })

  // The hidden-inset title bar loses the native double-click-to-zoom, so it is
  // re-implemented here. Guarded for the non-Wails test and browser context.
  async function toggleMaximise(): Promise<void> {
    try {
      if (typeof Window?.ToggleMaximise === 'function') await Window.ToggleMaximise()
    } catch (error) {
      // eslint-disable-next-line no-console -- expected outside Wails, so not a warning
      console.debug('Window maximise is unavailable outside Wails', error)
    }
  }

  return {
    mode,
    setMode,
    terminalActive,
    agentsActive,
    hubActive,
    settingsActive,
    closeSettings,
    terminalMounted,
    agentsMounted,
    onScreenSessionSlug,
    feedSidebarCollapsed,
    terminalSidebarCollapsed,
    agentsSidebarCollapsed,
    previewCollapsed,
    feedNavActive,
    sidebarCollapsed,
    canToggleSidebar,
    toggleSidebar,
    previewToggleCollapsed,
    canTogglePreview,
    previewUnseen,
    togglePreview,
    codeCanvas,
    viewCanvasScope,
    toggleMaximise,
  }
}
