<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Window } from '@wailsio/runtime'
import { useEventListener, useStorage } from '@vueuse/core'
import { useRoute, useRouter } from 'vue-router'
import TitleBar from './components/TitleBar.vue'
import ProfileRail from './components/ProfileRail.vue'
import SideBar from './components/SideBar.vue'
import FeedList from './components/FeedList.vue'
import DetailPane from './components/DetailPane.vue'
import ActionInputsDialog from './components/ActionInputsDialog.vue'
import CreateSessionDialog from './components/CreateSessionDialog.vue'
import NewSessionDialog from './components/NewSessionDialog.vue'
import ConfirmationDialog from './components/ConfirmationDialog.vue'
import CommandPalette from './components/CommandPalette.vue'
import ErrorDialog from './components/ErrorDialog.vue'
import ReportProblemDialog from './components/ReportProblemDialog.vue'
import WhatsNewDialog from './components/WhatsNewDialog.vue'
import ProfileSettingsView from './components/ProfileSettingsView.vue'
import SettingsView from './components/SettingsView.vue'
import FlowsView from './pipeline/components/FlowsView.vue'
import ActivityOverlay from './components/ActivityOverlay.vue'
import TasksOverlay from './components/TasksOverlay.vue'
import DeleteProfileModal from './components/DeleteProfileModal.vue'
import NewProfileModal from './components/NewProfileModal.vue'
import UnsavedFlowChangesModal from './components/UnsavedFlowChangesModal.vue'
import OnboardingScreen from './components/OnboardingScreen.vue'
import ToastStack from './components/ToastStack.vue'
import SequenceHint from './components/SequenceHint.vue'
import { useGitHubConnection } from './composables/useGitHubConnection'
import { useNotificationSettings } from './composables/useNotificationSettings'
import { useAgentWorkspaces } from './stores/useAgentWorkspaces'
import { useFirstRun } from './composables/useFirstRun'
import { useHiveSetup } from './composables/useHiveSetup'
import { useAgentCanvasRoute } from './composables/useAgentCanvasRoute'
import { useActivity } from './stores/useActivity'
import { useJobs } from './stores/useJobs'
import { useFeedState } from './composables/useFeedState'
import { useOpenModalCount } from './composables/useOpenModalCount'
import { useCommandPalette } from './composables/useCommands'
import { useErrorDialog } from './composables/useErrorDialog'
import { useReportDialog } from './composables/useReportDialog'
import { useDevTools } from './composables/useDevTools'
import { useReleaseNotes } from './composables/useReleaseNotes'
import { useNewSession } from './composables/useNewSession'
import { usePopupTerminal } from './stores/usePopupTerminal'
import { useTasks } from './stores/useTasks'
import { useTerminalSessions } from './stores/useTerminalSessions'
import { useTerminalFont } from './stores/useTerminalFont'
import {
  closeTerminalPane,
  closeTerminalWindow,
  focusTerminalFilter,
  focusTerminalPane,
  focusTerminalPaneDirection,
  focusTerminalTree,
  newTerminalWindow,
  selectTerminalWindow,
  splitTerminalPane,
  stepTerminalWindow,
  zoomTerminalPane,
} from './lib/terminalTree'
import { focusAgentsFilter, focusAgentsList, focusAgentsPane } from './lib/agentsTree'
import { useLaunchers } from './composables/useLaunchers'
import { useItemSessions } from './composables/useItemSessions'
import { useWailsEvent } from './composables/useWailsEvent'
import { comboFromEvent, SEQUENCE_TIMEOUT_MS, terminalEscapeCombo, useKeybindings } from './composables/useKeybindings'
import {
  commandById,
  commandPiercesPane,
  launcherActionID,
  launcherCommandID,
  terminalWindowPosition,
  type CommandContext,
} from './keybindings/catalog'
import { useAppPaletteRows } from './composables/useAppPaletteRows'
import { useFlowsSession } from './pipeline/composables/useFlowsSession'
import { isEditableTarget, isTerminalTarget } from './lib/isEditableTarget'
import {
  InstallUpdate,
  Status as UpdaterStatus,
} from '../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/updaterservice'
import {
  Feed,
  FindItems,
} from '../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import type {
  MenuBarNavigation,
  NotificationActivation,
  NotificationToast,
  UpdateInfo,
} from '../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import {
  isApplicationSettingsSection,
  isProfileSettingsSection,
  type ApplicationSettingsSection,
  type ProfileSettingsSection,
} from './router'
import type { SidebarSelection } from './types/feed'
import { kind } from './lib/itemPresentation'
import type { ActivityItemLink } from './lib/activityPresentation'

// Only true when Vite is serving in dev mode (under `wails3 dev`). The dev
// strip is that build's own chrome and never ships; the developer tools behind
// it can also be turned on in a shipped build, where the numbers they report
// are the ones that matter (ADR developer-tools-are-reachable-in-a-shipped-build-behind-a-setting), so their chunk is defined
// unconditionally and simply never fetched unless the pane opens.
const devMode = import.meta.env.DEV
const DevBar = devMode ? defineAsyncComponent(() => import('./components/DevBar.vue')) : null
const DevView = defineAsyncComponent(() => import('./components/DevView.vue'))
const { enabled: devToolsEnabled, resolve: resolveDevTools } = useDevTools()

// Async so xterm.js stays out of the initial bundle: terminal mode is opt-in
// and the hub must not pay for it at startup. Mounted only once the mode is
// first entered — after which it stays mounted, same as the pop-up below.
const TerminalMode = defineAsyncComponent(() => import('./components/TerminalMode.vue'))
// Same reason: the Agents area's session pane pulls in xterm.js too, and it
// ships behind experimental.agents.
const AgentsMode = defineAsyncComponent(() => import('./components/AgentsMode.vue'))
// Same reason, and mounted only once the pop-up is first asked for — after
// which it stays mounted, because hiding it must not end the shell inside it.
const PopupTerminal = defineAsyncComponent(() => import('./components/PopupTerminal.vue'))

const {
  status: githubStatus,
  connected: githubConnected,
  deviceFlow,
  card: connectCard,
  error: connectError,
  busy: connectBusy,
  startDeviceFlow,
  useTokenInstead,
  backToStart,
  submitToken,
} = useGitHubConnection()

const {
  permission: notificationPermission,
  requestingPermission,
  error: notificationError,
  requestPermission,
} = useNotificationSettings()

const {
  profiles,
  profilesLoaded,
  profilesError,
  activeProfile,
  activeProfileId,
  selection,
  items,
  sourceIcons,
  sourceImages,
  visibleItems,
  unreadCount,
  search,
  authors,
  authorFilter,
  loadError,
  itemSelectionActive,
  selectedItemIDs,
  selectedItems,
  selectionActions,
  enterItemSelection,
  toggleItemSelection,
  cancelItemSelection,
  copySelectedItemContents,
  invokeSelectionAction,
  selectedId,
  selectedItem,
  actions,
  pendingAction,
  actionRuns,
  sessionLaunchAction,
  sessionLaunchOptions,
  sessionLaunchBusy,
  sessionLaunchError,
  actionInputsAction,
  actionInputsBusy,
  actionInputsError,
  actionRerunConfirmation,
  actionRerunBusy,
  actionRerunError,
  unreadOnly,
  feedSort,
  setFeedSort,
  title,
  toasts,
  showToast,
  dismissToast,
  clearToasts,
  creatingProfile,
  createProfileError,
  renamingProfile,
  renameProfileError,
  togglingProfileId,
  toggleProfileError,
  deletingProfile,
  settingProfileImage,
  profileImageError,
  loadProfiles,
  createProfile,
  seedStarterFlow,
  renameProfile,
  setProfileEnabled,
  deleteProfile,
  setProfileImage,
  clearProfileImage,
  visibleArchivedItems,
  archivedExpanded,
  archivedCount,
  toggleArchivedSection,
  trashFilter,
  setTrashFilter,
  reorderFeeds,
  reorderProfiles,
  selectProfile,
  defaultSelection,
  selectSidebar,
  selectItem,
  openActionRun,
  selectNext,
  selectPrev,
  toggleUnread,
  markItemUnread,
  markingAllRead,
  markAllRead,
  unreadInScope,
  toggleArchive,
  toggleIgnored,
  loadEvents,
  refresh,
  refreshSources,
  refreshingSources,
  invokeAction,
  cancelActionRerun,
  confirmActionRerun,
  cancelSessionLaunch,
  submitSessionLaunch,
  cancelActionInputs,
  submitActionInputs,
  notWired,
  openUrl,
  openItemInBrowser,
  openSelectedInBrowser,
  copyItemLink,
  copyItemContents,
  runItemAction,
  hideWindow,
} = useFeedState()

// The feed-item kinds currently in the system — what the actions editor
// autocompletes and validates "applies to" against. kind() never returns
// empty (untyped items report DEFAULT_ITEM_KIND), so untyped items are
// offered as a target like any other kind.
const knownFeedTypes = computed(() =>
  [...new Set(items.value.map((item) => kind(item)))].sort((a, b) => a.localeCompare(b)),
)

const selectedEvents = ref([] as Awaited<ReturnType<typeof loadEvents>>)
let selectedEventsSeq = 0
watch(
  selectedItem,
  async (item) => {
    const seq = ++selectedEventsSeq
    selectedEvents.value = []
    if (!item) return
    try {
      const events = await loadEvents(item.id)
      // A slower request for the previously selected item must never replace the
      // timeline for the item currently visible in the detail pane.
      if (seq === selectedEventsSeq && selectedItem.value?.id === item.id) selectedEvents.value = events
    } catch (error) {
      if (seq === selectedEventsSeq && selectedItem.value?.id === item.id) {
        console.warn('Unable to load inbox item events', error)
      }
    }
  },
  { immediate: true },
)

// ── Flows session (hc-8ft4yhm6) ──────────────────────────────────────────────
// A profile IS a flow, so the flows canvas is a per-profile sub-view: it swaps
// the sidebar+main region while the spaces rail and titlebar stay mounted (so
// the user is never stranded — see the template). Reached from the sidebar's
// "Flows" pill / "Edit flow" footer and the ⌘K command; exited by selecting a
// profile in the spaces rail or the ⌘K "Back to feed" command.
//
// The session (useFlowsSession) is a module singleton shared with
// FlowsView.vue: it owns the pipeline editor state — which flow is being
// edited, its dirty draft, the flow listing — nothing more. Execution is the
// Go engine's (runtime.Engine) alone, and it keeps every enabled flow running
// with the canvas closed or another profile selected; that is why feeds keep
// updating regardless of what this session is doing. App.vue is the first
// caller, which makes the session app-lived rather than dependent on
// FlowsView mounting/unmounting.
const session = useFlowsSession()

// ── Route-driven navigation ────────────────────────────────────────────────
// The Wails webview uses hash history: routes survive asset:// hosting and
// native/browser back and forward controls traverse the same page stack. The
// shell (title bar + profile rail) stays mounted while this route selects its
// main page.
const router = useRouter()
const route = useRoute()
const { routeChatId, canvasRequested, canvasUnseen, syncCanvasQuery } = useAgentCanvasRoute()
const flowsActive = computed(() => route.name === 'flows')
const devActive = computed(() => devToolsEnabled.value && route.name === 'dev')
const applicationSettingsActive = computed(() => route.name === 'application-settings')
const profileSettingsActive = computed(() => route.name === 'profile-settings')
// Resolved against router.ts's section lists rather than a whitelist repeated
// here: the route already rejects an unknown :section, so anything that
// reaches this point and is not recognized is the absent-param case.
const applicationSettingsSection = computed<ApplicationSettingsSection>(() =>
  isApplicationSettingsSection(route.params.section) ? route.params.section : 'general',
)
const profileSettingsSection = computed<ProfileSettingsSection>(() =>
  isProfileSettingsSection(route.params.section) ? route.params.section : 'general',
)
const canGoBack = computed(() => {
  void route.fullPath
  return router.options.history.state.back !== null
})
const canGoForward = computed(() => {
  void route.fullPath
  return router.options.history.state.forward !== null
})

// Keep the selected backend profile and flow draft aligned with route params.
// Missing profile params occur only on the first /feed load; canonicalize that
// entry once profiles arrive so future history entries are self-contained.
watch(activeProfileId, (id) => {
  session.bindActiveFlow(id || undefined)
  const routeNeedsProfile = route.name === 'feed' || route.name === 'flows' || route.name === 'profile-settings'
  if (id && routeNeedsProfile && !route.params.profileId) {
    void router.replace({ name: route.name, params: { ...route.params, profileId: id }, query: route.query })
  }
})

let feedRouteSync = 0
watch(
  [profilesLoaded, () => route.fullPath],
  async ([loaded]) => {
    if (!loaded) return
    const sync = ++feedRouteSync
    const rawProfileId = route.params.profileId
    if (typeof rawProfileId !== 'string') return
    if (!profiles.value.some((profile) => profile.id === rawProfileId)) {
      if (activeProfileId.value) void router.replace({ name: 'feed', params: { profileId: activeProfileId.value } })
      return
    }
    if (rawProfileId !== activeProfileId.value) await selectProfile(rawProfileId)
    if (sync !== feedRouteSync || route.name !== 'feed') return

    const rawFeedId = route.query.feed
    const feedId =
      typeof rawFeedId === 'string' && activeProfile.value?.feeds.some((feed) => feed.id === rawFeedId)
        ? rawFeedId
        : null
    const wantsUnread = route.query.unread === '1'
    if (feedId) await selectSidebar({ type: 'feed', feedId })
    else if (route.query.view === 'trash') await selectSidebar({ type: 'trash' })
    // A bare feed route means "the profile default": last-selected feed,
    // else the first feed in sidebar order. Defaults are never persisted as
    // the remembered selection.
    else await selectSidebar(defaultSelection(rawProfileId), { persist: false })

    // Unread narrows whichever feed the route selected.
    // selectSidebar clears the flag, so apply it after loading that list.
    if (wantsUnread && !unreadOnly.value) await toggleUnread()

    // ?item reveals one row in whichever list the route just loaded. It is how
    // a clicked notification lands on its item, so the destination is a normal
    // route: back/forward traverse it like any other navigation.
    const wantedItem = Number(route.query.item)
    if (!Number.isSafeInteger(wantedItem) || wantedItem <= 0) return
    // An archived row lives in the lazily loaded section below the list, so
    // expand it before giving up on finding the item.
    if (!items.value.some((item) => item.id === wantedItem) && !archivedExpanded.value) {
      await toggleArchivedSection()
      if (sync !== feedRouteSync || route.name !== 'feed') return
    }
    await selectItem(wantedItem)
  },
  { immediate: true },
)

watch(
  [() => route.name, () => route.query.node],
  ([name, rawNode]) => {
    if (name === 'flows') session.openFlows(typeof rawNode === 'string' ? rawNode : undefined)
    else session.exitFlows()
  },
  { immediate: true },
)

// A router guard protects dirty flow drafts for every navigation source,
// including native mouse/browser Back — not only the app's own buttons.
type PendingNavigation = { to: string }
const pendingNavigation = ref<PendingNavigation | null>(null)
const unsavedChangesBusy = ref(false)
let allowGuardedNavigation = false
const removeNavigationGuard = router.beforeEach((to, from) => {
  const switchesProfile = typeof to.params.profileId === 'string' && to.params.profileId !== activeProfileId.value
  const leavesDirtyFlow =
    from.name === 'flows' && (to.name !== 'flows' || to.params.profileId !== from.params.profileId)
  if (!allowGuardedNavigation && (leavesDirtyFlow || switchesProfile) && session.dirty.value) {
    pendingNavigation.value = { to: to.fullPath }
    return false
  }
})
onUnmounted(removeNavigationGuard)

function cancelPendingNavigation(): void {
  pendingNavigation.value = null
}

async function finishPendingNavigation(): Promise<void> {
  const pending = pendingNavigation.value
  if (!pending) return
  pendingNavigation.value = null
  allowGuardedNavigation = true
  try {
    await router.push(pending.to)
  } finally {
    allowGuardedNavigation = false
  }
}

async function deployPendingNavigation(): Promise<void> {
  if (!pendingNavigation.value) return
  unsavedChangesBusy.value = true
  await session.deploy()
  unsavedChangesBusy.value = false
  if (session.dirty.value) return
  await finishPendingNavigation()
}

async function discardPendingNavigation(): Promise<void> {
  if (!pendingNavigation.value) return
  unsavedChangesBusy.value = true
  await session.discardDraft()
  unsavedChangesBusy.value = false
  if (session.dirty.value) return
  await finishPendingNavigation()
}

function openFeed(profileId = activeProfileId.value): void {
  void router.push({ name: 'feed', params: profileId ? { profileId } : {} })
}

function navigateSidebar(nextSelection: SidebarSelection): void {
  if (!activeProfileId.value) return
  const query = nextSelection.type === 'feed' ? { feed: nextSelection.feedId } : { view: 'trash' }
  void router.push({ name: 'feed', params: { profileId: activeProfileId.value }, query })
}

function navigateUnreadFilter(value: boolean): void {
  if (!activeProfileId.value) return
  const query: Record<string, string> = {}
  if (selection.value.type === 'feed') query.feed = selection.value.feedId
  else query.view = 'trash'
  if (value) query.unread = '1'
  void router.push({ name: 'feed', params: { profileId: activeProfileId.value }, query })
}

function navigateUnreadToggle(): void {
  navigateUnreadFilter(!unreadOnly.value)
}

function openFlows(focusNodeId?: string): void {
  if (!activeProfileId.value) return
  // Update immediately for command-palette and canvas focus feedback; the
  // route watcher keeps this state aligned during back/forward traversal.
  session.openFlows(focusNodeId)
  void router.push({
    name: 'flows',
    params: { profileId: activeProfileId.value },
    query: focusNodeId ? { node: focusNodeId } : {},
  })
}

function requestExitFlows(): void {
  openFeed()
}

async function requestSelectProfile(id: string): Promise<void> {
  // Returning from the canvas to the already-active profile does not change
  // the route profile id, so the route watcher will not reload its feeds.
  // Refresh explicitly after a clean deploy so the sidebar cannot retain the
  // pre-deploy flow snapshot if flows:updated races the filesystem watcher.
  if (id === activeProfileId.value && !session.dirty.value) await selectProfile(id)
  openFeed(id)
}

function requestOpenActionsSettings(): void {
  void router.push({ name: 'application-settings', params: { section: 'actions' } })
}

function requestOpenSettings(page: 'application' | 'profile'): void {
  if (page === 'application') void router.push({ name: 'application-settings' })
  else if (activeProfileId.value)
    void router.push({ name: 'profile-settings', params: { profileId: activeProfileId.value } })
}

// ── Activity (6d) ─────────────────────────────────────────────────────────────
// App-global audit log. The titlebar's Activity icon replaces the old "polling
// github" indicator; unseenActivity drives its dot.
//
// Activity is an overlay, not a route (hay-kot/hive-desktop#441) — the same shape as Tasks, so the
// audit log opens over whatever you were reading instead of navigating away
// from it. The icon toggles: clicking it while open closes it, matching the
// tint that communicates open state.
const { unseenCount: unseenActivity } = useActivity()
const { activeJobs, hasActive: jobsActive } = useJobs()
const { stepFontSize: stepTerminalFontSize, resetFontSize: resetTerminalFontSize } = useTerminalFont()
const { sessionRepository } = useTerminalSessions()

const activityOpen = ref(false)

function openActivity(): void {
  activityOpen.value = !activityOpen.value
}

// Tasks is an overlay, not a route, so the titlebar icon toggles it — clicking
// it while open closes it, matching the icon's tint communicating open state.
const tasksOpen = ref(false)
const { setRepoKey: setTasksRepoKey } = useTasks()
// The attached terminal session's resolved owner/repo, kept live by
// TerminalMode's continuous report rather than read only on click, so every
// way of opening Tasks — keybinding, palette, the status-bar button itself —
// scopes to it the same way.
const terminalSessionRepoKey = ref('')

// Every entry point funnels through this one toggle (see runMap's
// 'tasks.toggle' and TerminalMode's open-tasks emit). Only an
// *opening* click re-resolves the scope: closing must never move it, and a
// session with no resolved repo (or the hub, with none at all) leaves the
// persisted last-picked scope alone.
function openTasks(): void {
  const opening = !tasksOpen.value
  if (opening && terminalActive.value && terminalSessionRepoKey.value) {
    setTasksRepoKey(terminalSessionRepoKey.value)
  }
  tasksOpen.value = !tasksOpen.value
}

async function openJobRun(commandID: number): Promise<void> {
  const job = activeJobs.value.find((candidate) => candidate.commandId === commandID)
  if (!job || !activeProfileId.value) return
  await router.push({ name: 'feed', params: { profileId: activeProfileId.value } })
  if (route.name !== 'feed') return
  // The route watcher applied the profile default selection; the run opens
  // only if its item is present in that list.
  await openActionRun(Number(job.target), job.actionId, commandID)
}

// ── Desktop self-update ───────────────────────────────────────────────────────
// The UpdaterService checks the configured release-channel manifest for a
// newer desktop release in the background (when enabled) and emits update:available. We seed initial state
// via Status() on mount and keep it current through the subscription, so the
// title-bar chip appears without waiting for the next poll. Clicking it
// downloads + relaunches into the new version.
const updateInfo = ref<UpdateInfo | null>(null)
const updateAvailable = computed(() => updateInfo.value?.available ?? false)
const updateLatestVersion = computed(() => updateInfo.value?.latestVersion ?? '')
const installingUpdate = ref(false)
const updateConfirmOpen = ref(false)
const updateInstallError = ref('')

function openUpdate(): void {
  if (installingUpdate.value) return
  updateInstallError.value = ''
  updateConfirmOpen.value = true
}

function cancelUpdate(): void {
  if (installingUpdate.value) return
  updateConfirmOpen.value = false
  updateInstallError.value = ''
}

async function confirmUpdate(): Promise<void> {
  if (installingUpdate.value) return
  installingUpdate.value = true
  updateInstallError.value = ''
  try {
    await InstallUpdate()
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error)
    console.error('Update install failed', error)
    updateInstallError.value = detail
    showToast('Could not install the update', {
      severity: 'error',
      body: detail,
      duration: 10_000,
    })
    installingUpdate.value = false
  }
}

// ── Mark all as read ─────────────────────────────────────────────────────────
// Clearing one feed is scoped, visible in the sidebar, and the thing the user
// just asked for, so it runs straight away. The profile variant reaches every
// feed at once with no undo, so it confirms and names the count first.
const markWorkspaceReadOpen = ref(false)
const workspaceUnreadCount = computed(() => unreadInScope(null))

function markSelectedFeedRead(): void {
  // Trash carries no unread semantics, so there is nothing here to clear.
  if (selection.value.type !== 'feed') return
  void markAllRead(selection.value.feedId)
}

function requestMarkWorkspaceRead(): void {
  if (markingAllRead.value) return
  markWorkspaceReadOpen.value = true
}

async function confirmMarkWorkspaceRead(): Promise<void> {
  await markAllRead(null)
  markWorkspaceReadOpen.value = false
}

function closeSettings(): void {
  openFeed()
}

function selectApplicationSettingsSection(section: ApplicationSettingsSection): void {
  void router.push({ name: 'application-settings', params: { section } })
}

function selectProfileSettingsSection(section: ProfileSettingsSection): void {
  if (!activeProfileId.value) return
  void router.push({ name: 'profile-settings', params: { profileId: activeProfileId.value, section } })
}

// ── Titlebar error chip (8d) ──────────────────────────────────────────────────
// Sourced from the always-on session (not FlowsView), so the chip renders and
// deep-links correctly even with the canvas closed.
const errorNodeIds = computed(() =>
  (session.activeFlow.value?.nodes ?? [])
    .filter((node) => session.latestRunByNode.value.get(node.id)?.ok === false)
    .map((node) => node.id),
)
const errorCount = computed(() => errorNodeIds.value.length)
const firstErrorNodeId = computed(() => errorNodeIds.value[0])

function openErrorNode(): void {
  if (firstErrorNodeId.value) openFlows(firstErrorNodeId.value)
}

async function revealInboxItem(profileId: string, itemId: number): Promise<void> {
  let feedId = ''
  try {
    feedId = (await Feed(profileId, itemId)) ?? ''
  } catch (error) {
    console.warn('Unable to locate the inbox item', error)
  }
  const query: Record<string, string> = { item: String(itemId) }
  if (feedId) query.feed = feedId
  else query.view = 'trash'
  await router.push({ name: 'feed', params: { profileId }, query })
}

// A clicked notification arrives with the profile and item it was sent
// about (see the notify node). The window is already raised by the time this
// fires; routing to a feed route that reveals the item is all that is left.
// An item id of 0 means the notification had none — land on the profile.
async function revealNotification(activation: NotificationActivation): Promise<void> {
  const profileId = activation.profileId
  if (!profileId) return
  const itemId = Number(activation.itemId ?? 0)
  if (!Number.isSafeInteger(itemId) || itemId <= 0) {
    openFeed(profileId)
    return
  }
  await revealInboxItem(profileId, itemId)
}

// The window is raised before the menu bar event is emitted.
async function openFromMenuBar(nav: MenuBarNavigation): Promise<void> {
  if (nav.settings) {
    selectApplicationSettingsSection('menubar')
    return
  }
  if (!nav.profileId) return
  if (nav.itemId > 0) {
    await revealInboxItem(nav.profileId, nav.itemId)
    return
  }
  await router.push({
    name: 'feed',
    params: { profileId: nav.profileId },
    query: nav.feedId ? { feed: nav.feedId } : {},
  })
}

async function openActivityItem(link: ActivityItemLink): Promise<void> {
  try {
    const candidates = (await FindItems(link.profileId, link.externalId)) ?? []
    const exact = candidates.find(
      (item) => item.sourceKind === link.sourceKind && item.sourceScope === link.sourceScope,
    )
    const item = exact ?? (candidates.length === 1 ? candidates[0] : undefined)
    if (!item) {
      showToast('Could not find the linked item', { severity: 'error' })
      return
    }
    await revealInboxItem(link.profileId, item.id)
  } catch (error) {
    console.warn('Unable to open the linked activity item', error)
    showToast('Could not open the linked item', { severity: 'error' })
  }
}

// Go speaks the notify vocabulary (info/success/warning/error); the toast
// stack speaks its own. Anything unrecognized reads as info rather than
// being dropped — the message still matters.
function toastSeverity(severity: string): 'info' | 'success' | 'warning' | 'error' {
  const known = ['info', 'success', 'warning', 'error'] as const
  return known.includes(severity as (typeof known)[number]) ? (severity as (typeof known)[number]) : 'info'
}
onMounted(() => {
  // /dev is a real route in every build, so a shipped one that was not asked to
  // expose the tools sends it back to the feed once the gate answers.
  void resolveDevTools().then((allowed) => {
    if (!allowed && route.name === 'dev') void router.push({ name: 'feed' })
  })
  // Seed the update chip from the last cached check; update:available below
  // reacts to background checks. The event payload is the same UpdateInfo
  // shape Status() returns.
  void UpdaterStatus()
    .then((status) => {
      updateInfo.value = status
    })
    .catch((error) => {
      console.debug('Updater status unavailable', error)
    })
})
// The Go flow engine commits before it announces, so this is the moment
// membership claims and inbox items are readable — not log:appended, which
// only says a source observed something that may route nowhere at all.
useWailsEvent('inbox:updated', () => {
  void refresh()
})
// The app owns this subscription, rather than FlowsView, because the flow
// listing feeds the sidebar whether or not the canvas is open. The session
// keeps an unsaved editor draft private while refreshing the rest.
useWailsEvent('flows:updated', () => {
  void session.reloadFlows()
})
useWailsEvent('update:available', (event: { data: UpdateInfo | UpdateInfo[] }) => {
  const payload = Array.isArray(event.data) ? event.data[0] : event.data
  if (payload) updateInfo.value = payload
})
useWailsEvent('notification:activated', (event: { data: NotificationActivation | NotificationActivation[] }) => {
  const payload = Array.isArray(event.data) ? event.data[0] : event.data
  if (payload) void revealNotification(payload)
})
useWailsEvent('menubar:open', (event: { data: MenuBarNavigation | MenuBarNavigation[] }) => {
  const payload = Array.isArray(event.data) ? event.data[0] : event.data
  if (payload) void openFromMenuBar(payload)
})
// A flow notification the user chose to receive in-app rather than as an OS
// banner (Settings -> Notifications -> Delivery). Go has already applied the
// kill switch and picked this channel; the toast stack is the same one every
// other in-app notification uses.
useWailsEvent('notification:toast', (event: { data: NotificationToast | NotificationToast[] }) => {
  const payload = Array.isArray(event.data) ? event.data[0] : event.data
  if (payload?.title) showToast(payload.title, { body: payload.body, severity: toastSeverity(payload.severity) })
})

// ── Profile create / delete overlays ─────────────────────────────────────────

const newProfileOpen = ref(false)

function openNewProfile() {
  createProfileError.value = null // a stale failure must not greet the reopen
  newProfileOpen.value = true
}

async function submitNewProfile(name: string) {
  await createProfile(name)
  if (!createProfileError.value) {
    newProfileOpen.value = false
    openFeed(activeProfileId.value)
  }
}

async function submitProfileRename(name: string) {
  if (!activeProfileId.value) return
  await renameProfile(activeProfileId.value, name)
}

async function submitProfileEnabled(enabled: boolean) {
  if (!activeProfileId.value) return
  await setProfileEnabled(activeProfileId.value, enabled)
}

async function submitProfileImage(data: string) {
  if (!activeProfileId.value) return
  await setProfileImage(activeProfileId.value, data)
}

async function submitProfileClearImage() {
  if (!activeProfileId.value) return
  await clearProfileImage(activeProfileId.value)
}

const deleteProfileOpen = ref(false)

function openDeleteProfile() {
  deleteProfileOpen.value = true
}

async function confirmDeleteProfile() {
  if (!activeProfileId.value) return
  const deleted = await deleteProfile(activeProfileId.value)
  if (!deleted) return
  deleteProfileOpen.value = false
  openFeed()
}

// Profiles load at startup regardless of GitHub (useFeedState's onMounted) —
// nothing in the app is gated on being connected. This reload is about
// identity, not availability: a different account must never be shown the
// previous account's data.
watch(
  () => (githubConnected.value ? (githubStatus.value?.login ?? '') : null),
  (key) => {
    if (key !== null) void loadProfiles()
  },
)

// ── First run ────────────────────────────────────────────────────────────────
// hive setup -> connect GitHub -> notifications -> meet the agent. Hive setup
// goes first because it is the one answer the rest of the app reads back — the
// new session picker is built from it — and because abandoning it costs
// nothing that early: no credential stored. Connecting can be skipped past a
// warning. The hand-off to the agent is last because it needs everything
// before it: the agent it opens is the one the Hive step chose, and the
// profile it builds on is the one connecting seeded.
//
// A profile is not a step. One exists before the app opens
// (FlowsService.EnsureProfile), so nothing here asks for a name.
//
// The walk is gated on a persisted marker, not inferred: with a profile always
// present, nothing in the app's state says whether the agent hand-off has
// happened. Each step's flag is then the tail of one walk, and the steps that
// have their own signal (a usable config, a connected account, a resolved
// grant) skip themselves.
const hive = useHiveSetup()
const firstRun = useFirstRun()
const { startFirstRunChat } = useAgentWorkspaces()
const hiveStepDone = ref(false)
// The step runs when the read succeeded, the file is one this app may write,
// and the step has not been retired. A config this app could not read is not
// a reason to hold a first run in front of the whole app — Settings ▸ Hive
// CLI shows the parse error. Whether the config is usable picks which body
// the step renders, not whether it is up.
const hiveStepActive = computed(
  () => firstRun.completed.value === false && !!hive.setup.value && !hive.unreadable.value && !hiveStepDone.value,
)
const firstRunConnect = ref(false)
// The OS notification grant. Requesting it here is the only place onboarding
// pops the OS prompt; a user whose permission is already resolved never sees
// this step (advanceToPermissions gates on it).
const firstRunPermissions = ref(false)
const firstRunAgent = ref(false)
const firstRunAgentError = ref<string | null>(null)
const startingFirstRunAgent = ref(false)
const onboardingActive = computed(
  () => hiveStepActive.value || firstRunConnect.value || firstRunPermissions.value || firstRunAgent.value,
)

// The walk starts once every read it branches on has landed: the marker, the
// Hive config, and the GitHub status. Starting on a status that has not
// arrived would put the connect card up for an account that is connected.
const firstRunReady = computed(
  () => firstRun.completed.value !== null && hive.loaded.value && githubStatus.value !== null,
)
let firstRunStarted = false
watch(
  firstRunReady,
  (ready) => {
    if (!ready || firstRunStarted || firstRun.completed.value !== false) return
    firstRunStarted = true
    if (!hiveStepActive.value) advanceToConnect()
  },
  { immediate: true },
)

function advanceToConnect(): void {
  firstRunConnect.value = !githubConnected.value
  if (!firstRunConnect.value) advanceToPermissions()
}

// Move onto the permissions step, unless the OS decision is already made — a
// grant or a denial has nothing left to ask, so the hand-off comes next.
function advanceToPermissions(): void {
  firstRunPermissions.value = notificationPermission.value === 'not-requested'
  if (!firstRunPermissions.value) firstRunAgent.value = true
}

function finishPermissions(): void {
  firstRunPermissions.value = false
  firstRunAgent.value = true
}

async function submitHiveSetup(): Promise<void> {
  if (await hive.save()) finishHiveStep()
}

// Leaving the step — saved, confirmed, or skipped — ends it. A skip means "not
// now, and not here again": the repository picker's empty state is what points
// at Settings ▸ Hive CLI afterwards.
function finishHiveStep(): void {
  hiveStepDone.value = true
  advanceToConnect()
}

function skipConnectStep(): void {
  firstRunConnect.value = false
  advanceToPermissions()
}

// The hand-off opens the seeded Hive workspace on the interview chat. First
// run ends before the route changes so the Agents area is not gated behind
// the screen that is handing off to it.
async function startFirstRunAgent(): Promise<void> {
  startingFirstRunAgent.value = true
  firstRunAgentError.value = null
  try {
    const session = await startFirstRunChat()
    await finishFirstRun()
    await router.push({ name: 'agents', params: { workspace: session.workspace }, query: { chat: String(session.id) } })
  } catch (error) {
    firstRunAgentError.value = error instanceof Error && error.message ? error.message : 'Could not start the chat.'
  } finally {
    startingFirstRunAgent.value = false
  }
}

async function finishFirstRun(): Promise<void> {
  firstRunAgent.value = false
  await firstRun.complete()
}

// Only an empty profile takes the starter graph: a walk replayed on an
// install whose profile already has feeds must not try to append a second
// one. A refusal is reported here, where the backend's reason (no account,
// or more than one) is the most useful thing to show.
const seedingStarter = ref(false)
const canSeedStarter = computed(() => githubConnected.value && activeProfile.value?.nodes === 0)
const emptyWorkspaceHint = computed(() => {
  if (!githubConnected.value) {
    return 'This profile has no feeds, and no account is connected to fetch as. Connect one under Integrations, then wire a source into a feed.'
  }
  if (canSeedStarter.value) {
    return 'This profile has no feeds. Add the starter feeds, or open the flow editor to wire a source into one.'
  }
  return 'This profile has no feeds. Open the flow editor to wire a source into one.'
})

async function seedStarterIntoEmptyProfile(): Promise<boolean> {
  const profile = activeProfile.value
  if (!profile || profile.nodes !== 0 || seedingStarter.value) return false
  seedingStarter.value = true
  try {
    await seedStarterFlow(profile.id)
    return true
  } catch (error) {
    console.warn('Unable to seed the starter flow', error)
    showToast('Starter feeds were not added', {
      body:
        error instanceof Error && error.message
          ? error.message
          : 'This profile has no sources yet — add one in the flow editor.',
      severity: 'error',
    })
    return false
  } finally {
    seedingStarter.value = false
  }
}

// Connecting seeds the active profile. It was made empty because a source
// node names the account it fetches as and there was none; this is the
// moment there is one, whether the connect happens on the first-run card or
// later under Integrations. Only a disconnected → connected transition
// seeds: the status load at startup must not, or a profile someone emptied
// on purpose would refill on every launch. During first run the connect card
// stays up until the seed lands, so the feed is never rendered sourceless on
// the way through.
watch(githubStatus, async (status, previous) => {
  if (previous?.state !== 'disconnected' || status?.state !== 'connected') return
  const duringFirstRun = firstRunConnect.value
  try {
    const seeded = await seedStarterIntoEmptyProfile()
    if (seeded && !duringFirstRun) {
      showToast('Starter feeds added', { body: 'The active profile now has the starter GitHub feeds.' })
    }
  } finally {
    if (duringFirstRun) {
      firstRunConnect.value = false
      advanceToPermissions()
    }
  }
})

// ── App mode ─────────────────────────────────────────────────────────────────
// Inbox is the feed/flows/settings app; Code takes the whole frame under the
// title bar; Agents is the third area, holding named workspaces (spec-tracked
// as hc-49x3i833). Terminal and Agents are both routes (/terminal/:slug?,
// /workspaces/:workspace?), so the title-bar controls stay live inside them —
// Activity, back/forward — and history traversal restores where each was
// left. A relaunch still lands on the hub (the webview loads with no hash),
// so no relaunch attaches a tmux control client or an agent session
// unprompted; re-entering a mode is what resumes it.
const mode = computed<'hub' | 'terminal' | 'agents'>(() => {
  if (route.name === 'terminal') return 'terminal'
  if (route.name === 'agents') return 'agents'
  return 'hub'
})
// Neither mode is on screen behind the loading frame, and a deep link to
// /terminal or /workspaces must not mount the hub under it — the three are
// siblings, not branches of one chain, so hubActive is written as the
// positive case rather than "not terminal": a third route with no explicit
// case here would otherwise render the hub underneath it.
// Deliberately not gated on the GitHub status: nothing in the app waits on
// being connected. The first-run chain waits for it itself before choosing
// the connect card.
const shellLoaded = computed(
  () => (profilesLoaded.value || !!profilesError.value) && hive.loaded.value && firstRun.completed.value !== null,
)
const terminalActive = computed(() => mode.value === 'terminal' && shellLoaded.value && !onboardingActive.value)
const agentsActive = computed(() => mode.value === 'agents' && shellLoaded.value && !onboardingActive.value)
const hubActive = computed(() => mode.value === 'hub' && shellLoaded.value && !onboardingActive.value)

// Terminal mode is mounted on first entry and never unmounted: its pool holds
// live tmux control clients and xterm screens bound to the elements they were
// opened on, so tearing the mode down paid a full re-attach — process spawn,
// pane capture, scrollback replay, renderer claim — on the way back in, and
// the pool ADR terminal-attach-pool warms stopped dead at the mode boundary. Hiding it is a
// view change, not the end of the shell (same rule as PopupTerminal). The
// Agents area gets the same treatment for the same reason (ADR terminal-mode-is-hidden-not-unmounted): it
// holds live PTYs bound to the elements they were opened on, so unmounting on
// a trip to the hub would end every running session's pane. Its `active` prop
// — not mount — is what will drive phase 8's activity poll.
const terminalMounted = ref(false)
watch(
  terminalActive,
  (active) => {
    if (active) terminalMounted.value = true
  },
  { immediate: true },
)
const agentsMounted = ref(false)
watch(
  agentsActive,
  (active) => {
    if (active) agentsMounted.value = true
  },
  { immediate: true },
)

// Where each mode's toggle lands: the route that mode was last on, so a round
// trip is not a trip to the default feed — or, on the terminal/agents side, a
// pass through the picker on the way back to what was already open. The
// agents path alone survives a reload (localStorage): it restores the focus
// filter and the open chat, and ?chat reattaches only a still-live session
// (ADR the-open-chat-rides-the-route) — never a relaunch — while a restored /terminal/:slug would
// attach a tmux control client unconditionally, so terminal's stays
// in-memory.
let lastHubPath = ''
let lastTerminalPath = ''
const lastAgentsPath = useStorage('hive.mode.agents.path', '')
if (!lastAgentsPath.value.startsWith('/workspaces')) lastAgentsPath.value = ''
watch(
  () => route.fullPath,
  (path) => {
    if (!route.name) return
    if (route.name === 'terminal') lastTerminalPath = path
    else if (route.name === 'agents') lastAgentsPath.value = path
    else lastHubPath = path
  },
  { immediate: true },
)

function setMode(next: 'hub' | 'terminal' | 'agents'): void {
  if (next === mode.value) return
  if (next === 'terminal') void router.push(lastTerminalPath || { name: 'terminal' })
  else if (next === 'agents') void router.push(lastAgentsPath.value || { name: 'agents' })
  else void router.push(lastHubPath || { name: 'feed' })
}

// ── Layout chrome ─────────────────────────────────────────────────────────────
// Each mode remembers its own left panel while the feed detail preview remains
// feed-only. The title-bar toggle follows whichever mode owns the current panel.
const feedSidebarCollapsed = useStorage('hive.panel.sidebar.collapsed', false)
const terminalSidebarCollapsed = useStorage('hive.panel.terminal.sidebar.collapsed', false)
const agentsSidebarCollapsed = useStorage('hive.panel.agents.sidebar.collapsed', false)
const previewCollapsed = useStorage('hive.panel.detailpane.collapsed', false)
const feedViewActive = computed(
  () =>
    !onboardingActive.value &&
    !terminalActive.value &&
    !agentsActive.value &&
    !applicationSettingsActive.value &&
    !profileSettingsActive.value &&
    !flowsActive.value &&
    !devActive.value &&
    !!activeProfile.value,
)
// The flag the title-bar toggle drives: whichever mode owns the panel on
// screen, and null in a view that has no left panel at all (settings, flows),
// which is what disables the button. One mode test, not the same ternary in
// three places.
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

// The right-panel toggle names whichever pane sits on that edge: the detail
// preview in Inbox, the Chats canvas in Chats. One control per frame edge is
// what keeps panel behaviour the same everywhere; the canvas used to carry its
// own button in the pane status bar instead (hay-kot/hive-desktop#432).
const agentsCanvasAvailable = computed(() => agentsActive.value && routeChatId.value !== null)
const previewToggleCollapsed = computed(() =>
  agentsCanvasAvailable.value ? !canvasRequested.value : previewCollapsed.value,
)
const canTogglePreview = computed(() => feedViewActive.value || agentsCanvasAvailable.value)
const previewUnseen = computed(() => agentsCanvasAvailable.value && canvasUnseen.value)

function togglePreview(): void {
  if (agentsCanvasAvailable.value) {
    syncCanvasQuery(!canvasRequested.value)
    return
  }
  previewCollapsed.value = !previewCollapsed.value
}

// Activating a row — a double-click, or Enter/Space on the focused row — is an
// explicit request to read it, so it opens a collapsed pane. A single click
// only moves the selection, which leaves a closed pane closed: the first click
// of every double-click is one, so reopening on `select` would fire before the
// gesture the user is making has finished.
async function activateItemFromRow(id: number): Promise<void> {
  previewCollapsed.value = false
  await selectItem(id)
}

// We draw our own hidden-inset title bar, so the native double-click-to-zoom
// gesture has to be re-implemented. Guarded for the non-Wails test/browser
// context, matching hideWindow's posture.
async function toggleMaximise(): Promise<void> {
  try {
    if (typeof Window?.ToggleMaximise === 'function') await Window.ToggleMaximise()
  } catch (error) {
    console.debug('Window maximise is unavailable outside Wails', error)
  }
}

// ── Command palette ──────────────────────────────────────────────────────────

const { open: paletteOpen, toggle: togglePalette, openWithScope } = useCommandPalette()
const { open: reportDialogOpen, openBundleDialog, reportProblem } = useReportDialog()
const { current: appError, dismissError } = useErrorDialog()

// An update installs by relaunching, so the version bump is only observable on
// the next launch — that is where the What's New surface is triggered from.
const {
  dialogOpen: whatsNewOpen,
  pendingEntries: whatsNewEntries,
  pendingVersion: whatsNewVersion,
  checkOnLaunch: checkReleaseNotes,
  dismiss: dismissWhatsNew,
} = useReleaseNotes()
onMounted(() => {
  void checkReleaseNotes()
})
// The Hive config decides whether first run has a step at all, so it is read
// alongside the profiles rather than when the step would render — the shell
// holds its empty frame until profilesLoaded, and a step that resolved after
// that would flash in behind it.
onMounted(() => {
  void hive.load()
  void firstRun.load()
})
const {
  open: newSessionOpen,
  options: newSessionOptions,
  initial: newSessionInitial,
  initialTarget: newSessionInitialTarget,
  busy: newSessionBusy,
  error: newSessionError,
  failure: newSessionFailure,
  formKey: newSessionFormKey,
  openBlank: openNewSession,
  openFromItem: openNewSessionFromItem,
  openFromItems: openNewSessionFromItems,
  cancel: cancelNewSession,
  submit: submitNewSession,
  dismissFailure: dismissNewSessionFailure,
  onCreateFailed: onNewSessionFailed,
} = useNewSession()
const kb = useKeybindings()
const creatingFromSelection = ref(false)

async function openSelectedItemsSession(): Promise<void> {
  if (selectedItems.value.length === 0) return
  await openNewSessionFromItems(selectedItems.value)
  creatingFromSelection.value = newSessionOpen.value
}

function cancelNewSessionDialog(): void {
  creatingFromSelection.value = false
  cancelNewSession()
}

async function submitNewSessionAndClearSelection(input: {
  repository?: string
  workspace?: string
  name: string
  prompt: string
  agent?: string
}): Promise<void> {
  await submitNewSession(input)
  if (!newSessionOpen.value && creatingFromSelection.value) {
    creatingFromSelection.value = false
    cancelItemSelection()
  }
}

// The URL is the attach state, so it is also the answer to "which session is on
// screen" — the pop-up terminal, the launchers, and a new session all follow it.
const onScreenSessionSlug = computed(() =>
  route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
)

// The pop-up terminal opens where the terminal on screen is, and in the user's
// home when none is (ADR ephemeral-popup-terminals). The panel is mounted on first
// use and stays mounted: hiding it is a view change, not the end of the shell.
const popupTerminal = usePopupTerminal()
const popupTerminalMounted = ref(false)

function togglePopupTerminal(): void {
  popupTerminalMounted.value = true
  popupTerminal.toggle({ sessionSlug: onScreenSessionSlug.value || undefined })
}

// A launcher is the pop-up opened straight into a program, and unless it pins
// itself to a directory it opens where the terminal on screen is — which is what
// makes one chord mean "lazygit here". Outside terminal mode there is nothing
// for it to open in, so the launch is not attempted: the core refuses it anyway,
// and a pop-up that appeared only to report that is worse than one that never
// opened (ADR quick-terminal-launchers-are-session-scoped).
function toggleLauncher(actionID: string): void {
  const command = commandById.value.get(launcherCommandID(actionID))
  if (command && !contextActive(command.context)) return
  popupTerminalMounted.value = true
  popupTerminal.toggle({ launcher: actionID, sessionSlug: onScreenSessionSlug.value || undefined })
}

// The launchers are read here rather than by the panel: they are commands in
// the palette and the keymap whether or not a pop-up has ever been opened, so
// they have to be known before the first one is invoked.
const launchers = useLaunchers()
onMounted(() => {
  void launchers.refresh()
})
useWailsEvent('actions:updated', () => {
  void launchers.refresh()
})

// Driven off the selection rather than off selectItem, so every path that
// moves it — keyboard walk, a clicked notification, restoring a job's item —
// loads the same list.
const { sessions: itemSessions, load: loadItemSessions, refresh: refreshItemSessions } = useItemSessions()
watch(
  () => selectedItem.value?.id ?? null,
  (itemID) => {
    void loadItemSessions(itemID)
  },
  { immediate: true },
)
useWailsEvent('jobs:updated', () => {
  void refreshItemSessions()
})
// The dialog closed on submit, so the failure has to come to the user.
useWailsEvent('sessions:create-failed', () => {
  void onNewSessionFailed()
})

// Attaching is terminal mode's job; the route is the attach state (ADR terminal-transport),
// so linking through is a navigation and nothing here touches tmux.
function openItemSession(slug: string): void {
  void router.push({ name: 'terminal', params: { slug } })
}

// So view.focus-search can reach the feed's search box the same way
// TerminalMode.vue's own filter field is reached — through a handle, not a
// prop, since the command fires from the global keymap rather than a click.
const feedListRef = ref<InstanceType<typeof FeedList> | null>(null)

// One handler per bindable command id. Both the keydown dispatcher and the
// command palette run through this map, so each command has a single
// implementation and the palette can show its live shortcut.
const runMap: Record<string, () => void | Promise<void>> = {
  'feed.next': selectNext,
  'feed.prev': selectPrev,
  'feed.open-in-browser': openSelectedInBrowser,
  'feed.toggle-unread': navigateUnreadToggle,
  'feed.toggle-preview': togglePreview,
  'feed.refresh': refreshSources,
  'feed.toggle-selection': () => {
    itemSelectionActive.value ? cancelItemSelection() : enterItemSelection()
  },
  'feed.toggle-archive': async () => {
    if (selectedItem.value) await toggleArchive(selectedItem.value)
  },
  'feed.mark-unread': async () => {
    if (selectedItem.value) await markItemUnread(selectedItem.value, true)
  },
  'feed.mark-all-read': markSelectedFeedRead,
  'feed.mark-workspace-read': requestMarkWorkspaceRead,
  'palette.toggle': togglePalette,
  'report.open': () => {
    void reportProblem()
  },
  'report.bundle': openBundleDialog,
  'tasks.toggle': openTasks,
  'terminal.popup.toggle': togglePopupTerminal,
  'terminal.toggle-sidebar': toggleSidebar,
  // Reaching for the tree is also how you get a collapsed sidebar back: the
  // chord means "work in the session list", and a hidden panel is not an
  // answer to it.
  'terminal.focus-sidebar': () => {
    terminalSidebarCollapsed.value = false
    void nextTick(focusTerminalTree)
  },
  'terminal.focus-pane': focusTerminalPane,
  // One combo, dispatched on whichever surface is on screen — the feed's
  // search box and the session tree's filter are otherwise unrelated fields.
  'view.focus-search': () => {
    if (feedNavActive.value) void nextTick(() => feedListRef.value?.focusSearch())
    else if (terminalActive.value) {
      terminalSidebarCollapsed.value = false
      void nextTick(focusTerminalFilter)
    } else if (agentsActive.value) {
      agentsSidebarCollapsed.value = false
      void nextTick(focusAgentsFilter)
    }
  },
  'terminal.new-window': newTerminalWindow,
  'terminal.close-window': closeTerminalWindow,
  'terminal.next-window': () => stepTerminalWindow(1),
  'terminal.prev-window': () => stepTerminalWindow(-1),
  'terminal.split-right': () => splitTerminalPane('horizontal'),
  'terminal.split-down': () => splitTerminalPane('vertical'),
  'terminal.close-pane': closeTerminalPane,
  'terminal.zoom-pane': zoomTerminalPane,
  'terminal.text-size-increase': () => stepTerminalFontSize(1),
  'terminal.text-size-decrease': () => stepTerminalFontSize(-1),
  'terminal.text-size-reset': resetTerminalFontSize,
  'terminal.focus-pane-left': () => focusTerminalPaneDirection('left'),
  'terminal.focus-pane-right': () => focusTerminalPaneDirection('right'),
  'terminal.focus-pane-up': () => focusTerminalPaneDirection('up'),
  'terminal.focus-pane-down': () => focusTerminalPaneDirection('down'),
  // Same rule as terminal.focus-sidebar: the chord asks to work in the list,
  // so a hidden one comes back rather than swallowing the request.
  'agents.focus-sidebar': () => {
    agentsSidebarCollapsed.value = false
    void nextTick(focusAgentsList)
  },
  'agents.focus-pane': focusAgentsPane,
  'session.new': () => openNewSession(sessionRepository(onScreenSessionSlug.value)),
  'window.hide': hideWindow,
  'view.go-inbox': () => setMode('hub'),
  'view.go-code': () => setMode('terminal'),
  'view.go-chats': () => setMode('agents'),
  'settings.open': () => requestOpenSettings('application'),
  'history.back': () => router.back(),
  'history.forward': () => router.forward(),
  'palette.keys': () => openWithScope('keys'),
}

// Resolves a command id to its implementation. Launchers and the numbered
// window jumps are not in runMap: each is one implementation parameterised by
// what its id names — an action from actions.yml, a position in the window
// strip — rather than an entry per command.
function runCommand(id: string): void {
  const launcher = launcherActionID(id)
  if (launcher !== null) {
    toggleLauncher(launcher)
    return
  }
  const window = terminalWindowPosition(id)
  if (window !== null) {
    selectTerminalWindow(window)
    return
  }
  void runMap[id]?.()
}

// The feed only accepts bare navigation keys when it is actually the on-screen
// view (matches the condition under which <FeedList> renders below).
const feedNavActive = computed(
  () => route.name === 'feed' && !onboardingActive.value && !terminalActive.value && !!activeProfile.value,
)

function contextActive(context: CommandContext): boolean {
  switch (context) {
    case 'feed':
      return feedNavActive.value
    case 'terminal':
      return terminalActive.value
    // Terminal mode with nothing attached is the session picker, and a command
    // that runs where a terminal is has no more to work with there than it does
    // on the feed. Any attached slug qualifies, hive session or not.
    case 'terminal-session':
      return terminalActive.value && !!onScreenSessionSlug.value
    case 'agents':
      return agentsActive.value
    case 'sidebar':
      return canToggleSidebar.value
    // The Code half excludes the session picker for terminal-session's reason:
    // with no emulator drawn, the chord would change a size nobody can see.
    case 'any-terminal':
      return (terminalActive.value && !!onScreenSessionSlug.value) || agentsActive.value || popupTerminal.visible.value
    case 'global':
      return true
  }
}

// Every overlay except the tasks one — split out so onGlobalKeydown can let
// tasks.toggle close the tasks overlay specifically, while it still stays
// suppressed under any of these (report, new-profile, a confirm, ...), same
// as every other command.
const otherOverlayOpen = computed(
  () =>
    paletteOpen.value ||
    reportDialogOpen.value ||
    newProfileOpen.value ||
    deleteProfileOpen.value ||
    markWorkspaceReadOpen.value ||
    newSessionOpen.value ||
    activityOpen.value ||
    !!sessionLaunchAction.value ||
    !!actionInputsAction.value ||
    !!pendingNavigation.value,
)
// While an overlay owns the screen, only the palette toggle stays live —
// tasks.toggle gets its own narrower exception below.
const anyOverlayOpen = computed(() => otherOverlayOpen.value || tasksOpen.value)
// A BaseModal-backed confirm stacked inside the tasks overlay (delete, prune,
// ...) must keep tasks.toggle from also closing the overlay underneath it.
const openModalCount = useOpenModalCount()

// Everything the palette lists lives in this composable — catalog commands,
// mode switches, and the hub's own objects, plus the Go-to rows (sessions,
// windows, settings sections, chats) that are global rather than tied to a
// lazily mounted mode. App.vue keeps the dispatcher (runCommand, runMap) and
// hands over the narrow bundle of state and functions the rows need.
useAppPaletteRows({
  runCommand,
  contextActive,
  mode,
  shellLoaded,
  onboardingActive,
  hubActive,
  devToolsEnabled,
  router,
  profiles,
  activeProfile,
  requestSelectProfile,
  navigateSidebar,
  selectedItem,
  selectedItemIDs,
  openSelectedItemsSession,
  actions,
  invokeAction,
  flowsActive,
  openFlows,
  requestExitFlows,
  openNewProfile,
  onScreenSessionSlug,
})

// ── Global input navigation ──────────────────────────────────────────────────
// Resolves a keydown against the configurable keymap and runs the matched
// command. Bare (modifier-less) keys are ignored while typing; feed commands
// only fire on the feed; overlays suppress everything but the palette toggle.

// One timer at a time, for a sequence prefix that is also a complete binding
// (Zed's prefix rule): a continuation cancels it, and its fire dispatches
// through the same gate as everything else below rather than trusting the
// state that was true when it was armed.
let sequenceTimer: ReturnType<typeof setTimeout> | null = null

function cancelSequenceTimer(): void {
  if (sequenceTimer === null) return
  clearTimeout(sequenceTimer)
  sequenceTimer = null
}

function resetSequence(): void {
  cancelSequenceTimer()
  kb.clearPendingSequence()
}

// The gate an ordinary dispatch applies, factored out so the deferred
// sequence timer's fire runs it too.
function dispatchIfActive(id: string): boolean {
  const command = commandById.value.get(id)
  if (!command) return false
  if (anyOverlayOpen.value && id !== 'palette.toggle') {
    // tasks.toggle has to reach the dispatcher while its own overlay owns the
    // screen — that is what lets it close again — but only that overlay: a
    // different modal (report, new-profile, a confirm stacked inside Tasks
    // itself) still swallows it like any other command.
    const closesTasksOverlay =
      id === 'tasks.toggle' && tasksOpen.value && !otherOverlayOpen.value && openModalCount.value === 0
    if (!closesTasksOverlay) return false
  }
  if (!contextActive(command.context)) return false
  runCommand(id)
  return true
}

function armSequenceTimer(deferredCommandId: string): void {
  sequenceTimer = setTimeout(() => {
    sequenceTimer = null
    kb.clearPendingSequence()
    dispatchIfActive(deferredCommandId)
  }, SEQUENCE_TIMEOUT_MS)
}

// A sequence started before the palette opened — by a chord, or by a mouse
// click, which never reaches stepSequence at all — has nowhere to go once it
// does; onGlobalKeydown's own swallow case gives Esc the same treatment.
watch(paletteOpen, (open) => {
  if (open) resetSequence()
})

function onGlobalKeydown(e: KeyboardEvent): void {
  // The exceptions to the rule below, which hands a focused terminal every key.
  // Both kinds still answer to their own context, and an overlay suppresses
  // them as it does every global command.
  if (isTerminalTarget(e.target) && !kb.recording.value && !anyOverlayOpen.value) {
    // The commands the catalog marks `piercesPane` are claimed on the binding
    // alone: the pop-up toggle and Tasks, because the combo that opens an
    // overlay has to close it; `terminal.focus-sidebar` and its Chats twin,
    // because reaching the list is the pane's way out — only that half of the
    // focus pair, since the chord moving focus *into* a pane is unreachable
    // from inside one; and the numbered window jumps, only ever wanted from
    // inside the window being left.
    //
    // A launcher pierces for the pop-up's reason without being in the static
    // catalog, and answers to the context it carries there: one that opens where
    // its terminal is is not dispatched with no terminal attached, so its chord
    // falls through to whatever else would have taken it rather than opening a
    // terminal the program inside cannot use (ADR quick-terminal-launchers-are-session-scoped).
    const id = kb.resolve(comboFromEvent(e) ?? '')
    const pierces = !!id && (commandPiercesPane(id) || launcherActionID(id) !== null)
    if (id && pierces && contextActive(commandById.value.get(id)?.context ?? 'global')) {
      resetSequence()
      e.preventDefault()
      runCommand(id)
      return
    }
    // The commands the catalog marks `escapesPane` — the palette, the sidebar
    // toggle, and the window lifecycle — fire over a pane too, but only on
    // modifiers a terminal cannot use, which is what terminalEscapeCombo
    // answers. A bare Ctrl+K stays with the pane; it is readline's
    // kill-to-end-of-line, and Ctrl+T is its transpose.
    const escaped = kb.resolve(terminalEscapeCombo(e) ?? '')
    const command = escaped ? commandById.value.get(escaped) : undefined
    if (escaped && command?.escapesPane && contextActive(command.context)) {
      e.preventDefault()
      runCommand(escaped)
      return
    }
  }

  // A focused terminal owns every key, modifiers included, so tmux prefixes
  // reach the pane instead of firing a Hive shortcut. It cannot host a pending
  // sequence either — the pane would swallow whatever completes it — so
  // landing here (or the focusin listener below, for a focus change that
  // isn't a keystroke) always clears one.
  if (isTerminalTarget(e.target)) {
    resetSequence()
    return
  }

  // WebKit can treat an unhandled Backspace as browser Back. Suppress that
  // default outside editors while still allowing components such as the flow
  // canvas to use Backspace for their own actions.
  if (e.key === 'Backspace' && !isEditableTarget(e.target)) e.preventDefault()

  if (kb.recording.value) return // the settings editor is capturing this key
  const combo = comboFromEvent(e)
  if (!combo) return

  const transition = kb.stepSequence(kb.pendingSequence.value, combo)
  switch (transition.kind) {
    case 'run':
      resetSequence()
      if (dispatchIfActive(transition.commandId)) e.preventDefault()
      return
    case 'extend': {
      // A sequence can only start outside an editable field and outside an
      // overlay. Continuing one already pending is unaffected: by the time
      // either is open, whatever got it there has already cleared pending —
      // a completed run, an ordinary dispatch below, the palette watch above,
      // or a focus change into the field (onWindowFocusIn).
      //
      // A discarded start falls through to the dispatch below rather than
      // returning: the combo may also be a complete binding in its own right
      // (Zed's prefix rule), and that exact binding still has to fire — a bare
      // leader then hits the editable bare-key bail and types normally, same
      // as if it had never been a prefix of anything.
      const isStart = kb.pendingSequence.value === null
      if (isStart && (isEditableTarget(e.target) || anyOverlayOpen.value)) break
      cancelSequenceTimer()
      kb.pendingSequence.value = transition.pending
      e.preventDefault()
      if (transition.deferredCommandId) armSequenceTimer(transition.deferredCommandId)
      return
    }
    case 'swallow':
      resetSequence()
      e.preventDefault()
      return
    case 'pass':
      if (kb.pendingSequence.value) resetSequence()
      break
  }

  const id = kb.resolve(combo)
  if (!id) return

  const mods = combo.split('+')
  const hasModifier = mods.includes('mod') || mods.includes('ctrl') || mods.includes('alt')
  if (isEditableTarget(e.target) && !hasModifier) return

  if (dispatchIfActive(id)) e.preventDefault()
}

// A pane owns every key while it has focus, and an editable field owns the
// next keystroke, so a sequence cannot survive a focus change into either
// even without an intervening keystroke (a mouse click into the field or
// pane, or a focus change made programmatically).
function onWindowFocusIn(e: FocusEvent): void {
  if (isTerminalTarget(e.target) || isEditableTarget(e.target)) resetSequence()
}

function isHistoryMouseButton(e: MouseEvent): boolean {
  return e.button === 3 || e.button === 4
}

// MouseEvent buttons 3 and 4 are conventionally Browser Back and Browser
// Forward. Handle them through Vue Router so they use the same dirty-flow
// guard as the title-bar controls. Cancel both press and auxiliary-click
// defaults because WebViews differ on which event performs native navigation.
function preventNativeMouseHistory(e: MouseEvent): void {
  if (isHistoryMouseButton(e)) e.preventDefault()
}

function onGlobalMouseUp(e: MouseEvent): void {
  if (!isHistoryMouseButton(e)) return
  e.preventDefault()
  if (e.button === 3) router.back()
  else router.forward()
}

useEventListener(window, 'keydown', onGlobalKeydown)
useEventListener(window, 'focusin', onWindowFocusIn)
useEventListener(window, ['mousedown', 'auxclick'], preventNativeMouseHistory)
useEventListener(window, 'mouseup', onGlobalMouseUp)
onUnmounted(cancelSequenceTimer)
</script>

<template>
  <main class="h-screen w-screen overflow-hidden bg-app text-text">
    <div class="flex h-full min-h-0 flex-col overflow-hidden">
      <TitleBar
        :profile-name="onboardingActive ? undefined : (activeProfile?.name ?? 'Loading')"
        :mode="mode"
        :activity-active="activityOpen"
        :error-count="errorCount"
        :unseen-activity="unseenActivity"
        :jobs-active="jobsActive"
        :active-jobs="activeJobs"
        :update-available="updateAvailable"
        :update-installing="installingUpdate"
        :latest-version="updateLatestVersion"
        :can-go-back="canGoBack"
        :can-go-forward="canGoForward"
        :sidebar-collapsed="sidebarCollapsed"
        :can-toggle-sidebar="canToggleSidebar"
        :preview-collapsed="previewToggleCollapsed"
        :can-toggle-preview="canTogglePreview"
        :preview-unseen="previewUnseen"
        @set-mode="setMode"
        @back="router.back()"
        @forward="router.forward()"
        @open-error-node="openErrorNode"
        @open-activity="openActivity"
        @open-job-run="openJobRun"
        @open-update="openUpdate"
        @toggle-sidebar="toggleSidebar"
        @toggle-preview="togglePreview"
        @open-palette="togglePalette"
        @toggle-maximise="toggleMaximise"
      />
      <!-- Hold an empty frame until the profiles resolve so a returning user
           never sees onboarding flash by. A load failure falls through to the
           shell below, which renders the error with a retry. -->
      <div v-if="!shellLoaded" class="flex min-h-0 flex-1 items-center justify-center font-mono text-xs text-text-4">
        Loading…
      </div>
      <OnboardingScreen
        v-else-if="onboardingActive"
        :card="hiveStepActive ? 'hive' : firstRunConnect ? connectCard : firstRunPermissions ? 'permissions' : 'agent'"
        :device-flow="deviceFlow"
        :error="
          hiveStepActive
            ? hive.error.value
            : firstRunConnect
              ? connectError
              : firstRunPermissions
                ? notificationError
                : firstRunAgentError
        "
        :busy="
          hiveStepActive
            ? hive.saving.value
            : firstRunConnect
              ? connectBusy
              : firstRunPermissions
                ? requestingPermission
                : startingFirstRunAgent
        "
        :github-connected="githubConnected"
        :permission="notificationPermission"
        :hive="hive"
        @save-hive="submitHiveSetup"
        @finish-hive="finishHiveStep"
        @start-device-flow="startDeviceFlow"
        @use-token-instead="useTokenInstead"
        @back-to-start="backToStart"
        @submit-token="submitToken"
        @skip-connect="skipConnectStep"
        @request-permission="requestPermission"
        @finish-permissions="finishPermissions"
        @start-agent="startFirstRunAgent"
        @finish-agent="finishFirstRun"
      />
      <!-- Terminal mode takes the whole frame under the title bar, spaces rail
           included: nothing in it is workspace-scoped, and the always-live mode
           toggle is the way back.

           v-show, not a branch of the chain above: leaving the mode must hide
           it, never unmount it — see terminalMounted. -->
      <TerminalMode
        v-if="terminalMounted"
        v-show="terminalActive"
        :active="terminalActive"
        :sidebar-collapsed="terminalSidebarCollapsed"
        @open-tasks="openTasks"
        @session-repo-key="terminalSessionRepoKey = $event"
      />
      <!-- Same treatment as terminal mode, for the same reason (ADR terminal-mode-is-hidden-not-unmounted):
           mount-once, hidden with v-show rather than unmounted. -->
      <AgentsMode
        v-if="agentsMounted"
        v-show="agentsActive"
        :active="agentsActive"
        :sidebar-collapsed="agentsSidebarCollapsed"
      />
      <!-- The spaces rail (ProfileRail) and TitleBar stay mounted across the
           feed<->flows switch; only the sidebar+main region swaps. This is
           what keeps the user from being stranded in the flows canvas — the
           spaces rail is always there to navigate back. -->
      <div v-if="hubActive" class="flex min-h-0 flex-1">
        <ProfileRail
          :profiles="profiles"
          :active-profile-id="activeProfileId"
          @select="requestSelectProfile"
          @add="openNewProfile"
          @reorder="reorderProfiles"
          @open-settings="requestOpenSettings('application')"
        />
        <DevView v-if="devActive" @close="closeSettings" />
        <SettingsView
          v-else-if="applicationSettingsActive"
          :active-category="applicationSettingsSection"
          :known-feed-types="knownFeedTypes"
          @close="closeSettings"
          @select-category="selectApplicationSettingsSection"
        />
        <ProfileSettingsView
          v-else-if="profileSettingsActive && activeProfile"
          :profile="activeProfile"
          :active-section="profileSettingsSection"
          :renaming="renamingProfile"
          :rename-error="renameProfileError"
          :toggling="togglingProfileId !== null"
          :toggle-error="toggleProfileError"
          :setting-image="settingProfileImage"
          :image-error="profileImageError"
          @close="closeSettings"
          @rename="submitProfileRename"
          @toggle-enabled="submitProfileEnabled"
          @set-image="submitProfileImage"
          @clear-image="submitProfileClearImage"
          @delete="openDeleteProfile"
          @select-section="selectProfileSettingsSection"
        />
        <FlowsView v-else-if="flowsActive" />
        <template v-else>
          <SideBar
            v-if="activeProfile && !feedSidebarCollapsed"
            :profile="activeProfile"
            :selection="selection"
            :flows-dirty="session.dirty.value"
            @select="navigateSidebar"
            @open-flows="openFlows()"
            @open-settings="requestOpenSettings('profile')"
            @reorder="(t) => activeProfile && reorderFeeds(activeProfile.id, t)"
          />
          <!-- A profile created before an account was connected has no
               graph at all, so there is no feed to render. Say what is
               missing and where to fix it rather than showing an empty Trash
               view, which is where a feedless flow otherwise lands.
               `tree` is what tells "this flow has no feed nodes" apart from
               "the feeds have not been read yet": a stub whose feeds a reload
               is still fetching has no tree, and must not flash this. -->
          <div
            v-if="activeProfile?.tree && activeProfile.feeds.length === 0"
            class="flex min-w-0 flex-1 flex-col items-center justify-center gap-3 px-10 text-center"
            data-testid="workspace-empty"
          >
            <div class="text-[13.5px] font-semibold">No sources yet</div>
            <p class="max-w-[400px] text-xs leading-relaxed text-text-3">{{ emptyWorkspaceHint }}</p>
            <div class="mt-1 flex items-center gap-2">
              <button
                v-if="canSeedStarter"
                class="cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text disabled:cursor-default disabled:opacity-60"
                data-testid="workspace-empty-seed"
                :disabled="seedingStarter"
                @click="seedStarterIntoEmptyProfile()"
              >
                Add starter feeds
              </button>
              <button
                v-if="!githubConnected"
                class="cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
                data-testid="workspace-empty-integrations"
                @click="selectApplicationSettingsSection('integrations')"
              >
                Open Integrations
              </button>
              <button
                class="cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
                data-testid="workspace-empty-flows"
                @click="openFlows()"
              >
                Edit flow
              </button>
            </div>
          </div>
          <section v-else-if="activeProfile" class="flex min-w-0 flex-1">
            <FeedList
              ref="feedListRef"
              :title="title"
              :visible-items="visibleItems"
              :selected-id="selectedId"
              :unread-only="unreadOnly"
              :unread-count="unreadCount"
              :archived-items="visibleArchivedItems"
              :archived-count="archivedCount"
              :archived-expanded="archivedExpanded"
              :trash="selection.type === 'trash'"
              :trash-filter="trashFilter"
              :search="search"
              :authors="authors"
              :author-filter="authorFilter"
              :sort="feedSort"
              :load-error="loadError"
              :refreshing="refreshingSources"
              :selection-mode="itemSelectionActive"
              :selected-item-ids="selectedItemIDs"
              :selection-actions="selectionActions"
              :source-icons="sourceIcons"
              :source-images="sourceImages"
              @select="selectItem"
              @activate="activateItemFromRow"
              @update:search="(value) => (search = value)"
              @update:author-filter="(value) => (authorFilter = value)"
              @set-sort="setFeedSort"
              @set-unread="navigateUnreadFilter"
              @toggle-archived="toggleArchivedSection"
              @set-trash-filter="setTrashFilter"
              @refresh="refreshSources"
              @mark-all-read="markSelectedFeedRead"
              @enter-selection="enterItemSelection"
              @toggle-item-selection="toggleItemSelection"
              @cancel-selection="cancelItemSelection"
              @copy-selection-contents="copySelectedItemContents"
              @run-selection-action="invokeSelectionAction"
              @create-session-from-selection="openSelectedItemsSession"
              @item-set-unread="markItemUnread"
              @item-toggle-archive="toggleArchive"
              @item-toggle-ignored="toggleIgnored"
              @item-open-browser="openItemInBrowser"
              @item-copy-link="copyItemLink"
              @item-copy-contents="copyItemContents"
              @item-create-session="openNewSessionFromItem"
              @item-run-action="runItemAction"
            />
            <DetailPane
              v-if="!previewCollapsed"
              :item="selectedItem"
              :events="selectedEvents"
              :actions="actions"
              :sessions="itemSessions"
              :pending-action="pendingAction"
              :action-runs="actionRuns"
              :source-icons="sourceIcons"
              :source-images="sourceImages"
              @run-action="invokeAction"
              @open-browser="openSelectedInBrowser"
              @open-url="openUrl"
              @set-unread="(value) => selectedItem && markItemUnread(selectedItem, value)"
              @toggle-archive="selectedItem && toggleArchive(selectedItem)"
              @toggle-ignored="selectedItem && toggleIgnored(selectedItem)"
              @copy-link="selectedItem && copyItemLink(selectedItem)"
              @copy-contents="selectedItem && copyItemContents(selectedItem)"
              @create-session="(target) => selectedItem && openNewSessionFromItem(selectedItem, target)"
              @open-session="openItemSession"
              @edit="requestOpenActionsSettings"
            />
          </section>
          <div v-else class="flex flex-1 flex-col items-center justify-center gap-3 font-mono text-xs text-text-4">
            <template v-if="profilesError">
              <span data-testid="profiles-error">{{ profilesError }}</span>
              <button
                class="cursor-pointer rounded border border-strong px-3 py-1.5 text-text-2 hover:text-text"
                @click="loadProfiles"
              >
                Retry
              </button>
            </template>
            <span v-else>Loading feed…</span>
          </div>
        </template>
      </div>
      <DevBar v-if="devMode" />
      <SequenceHint />
    </div>
    <CreateSessionDialog
      v-if="sessionLaunchAction && sessionLaunchOptions"
      :action-label="sessionLaunchAction.label"
      :options="sessionLaunchOptions"
      :inputs="sessionLaunchAction.inputs ?? []"
      :busy="sessionLaunchBusy"
      :error="sessionLaunchError"
      @close="cancelSessionLaunch"
      @submit="submitSessionLaunch"
    />
    <ActionInputsDialog
      v-if="actionInputsAction"
      :action-label="actionInputsAction.label"
      :inputs="actionInputsAction.inputs ?? []"
      :busy="actionInputsBusy"
      :error="actionInputsError"
      :submit-label="actionInputsAction.type === 'clipboard' ? 'Copy' : 'Run'"
      @close="cancelActionInputs"
      @submit="submitActionInputs"
    />
    <NewSessionDialog
      v-if="newSessionOpen && newSessionOptions"
      :key="newSessionFormKey"
      :options="newSessionOptions"
      :initial="newSessionInitial"
      :initial-target="newSessionInitialTarget"
      :busy="newSessionBusy"
      :error="newSessionError"
      :failure="newSessionFailure"
      @close="cancelNewSessionDialog"
      @submit="submitNewSessionAndClearSelection"
      @dismiss-failure="dismissNewSessionFailure"
    />
    <ConfirmationDialog
      v-if="updateConfirmOpen"
      title="Install update?"
      :description="`Download Hive ${updateLatestVersion || 'update'} and relaunch the app now?`"
      confirm-label="Install and relaunch"
      :busy="installingUpdate"
      :error="updateInstallError"
      testid="update-confirmation"
      @confirm="confirmUpdate"
      @cancel="cancelUpdate"
    />
    <ConfirmationDialog
      v-if="actionRerunConfirmation"
      title="Run action again?"
      :description="`${actionRerunConfirmation.label} has already run for this item. Run it again?`"
      confirm-label="Run again"
      :busy="actionRerunBusy"
      :error="actionRerunError"
      testid="action-rerun-confirmation"
      @confirm="confirmActionRerun"
      @cancel="cancelActionRerun"
    />
    <ConfirmationDialog
      v-if="markWorkspaceReadOpen"
      title="Mark all feeds as read?"
      :description="
        workspaceUnreadCount === 1
          ? `Clear the unread item in every feed of ${activeProfile?.name ?? 'this profile'}. This can't be undone.`
          : `Clear all ${workspaceUnreadCount} unread items in every feed of ${activeProfile?.name ?? 'this profile'}. This can't be undone.`
      "
      confirm-label="Mark all as read"
      :busy="markingAllRead"
      testid="mark-workspace-read-confirmation"
      @confirm="confirmMarkWorkspaceRead"
      @cancel="markWorkspaceReadOpen = false"
    />
    <ToastStack :toasts="toasts" @dismiss="dismissToast" @clear-all="clearToasts" />
    <PopupTerminal v-if="popupTerminalMounted" />
    <CommandPalette />
    <ReportProblemDialog v-if="reportDialogOpen" @close="reportDialogOpen = false" />
    <ErrorDialog v-if="appError" :error="appError" @close="dismissError" />
    <WhatsNewDialog
      v-if="whatsNewOpen"
      :version="whatsNewVersion"
      :entries="whatsNewEntries"
      @close="dismissWhatsNew"
    />
    <NewProfileModal
      v-if="newProfileOpen"
      :busy="creatingProfile"
      :error="createProfileError"
      @close="newProfileOpen = false"
      @create="submitNewProfile"
    />
    <DeleteProfileModal
      v-if="deleteProfileOpen && activeProfile"
      :profile-name="activeProfile.name"
      :busy="deletingProfile"
      @close="deleteProfileOpen = false"
      @confirm="confirmDeleteProfile"
    />
    <TasksOverlay v-if="tasksOpen" @close="tasksOpen = false" />
    <ActivityOverlay
      v-if="activityOpen"
      @close="activityOpen = false"
      @open-url="openUrl"
      @open-item="openActivityItem"
    />
    <!-- Deploying from this modal can raise the error dialog. Only one is
         rendered at a time: BaseModal closes on any Escape, so stacked
         overlays would both take a single keypress and drop the guard along
         with the error. Dismissing the error brings the guard back. -->
    <UnsavedFlowChangesModal
      v-if="pendingNavigation && !appError"
      :busy="unsavedChangesBusy"
      :error="session.error.value"
      @close="cancelPendingNavigation"
      @deploy="deployPendingNavigation"
      @discard="discardPendingNavigation"
    />
  </main>
</template>
