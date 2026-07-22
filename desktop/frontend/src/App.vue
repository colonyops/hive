<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Events, Window } from '@wailsio/runtime'
import { useStorage } from '@vueuse/core'
import { useRoute, useRouter } from 'vue-router'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import IconList from '~icons/lucide/list'
import IconPalette from '~icons/lucide/palette'
import IconRss from '~icons/lucide/rss'
import IconShare2 from '~icons/lucide/share-2'
import IconWorkflow from '~icons/lucide/workflow'
import TitleBar from './components/TitleBar.vue'
import ProfileRail from './components/ProfileRail.vue'
import SideBar from './components/SideBar.vue'
import FeedList from './components/FeedList.vue'
import DetailPane from './components/DetailPane.vue'
import CreateSessionDialog from './components/CreateSessionDialog.vue'
import CommandPalette from './components/CommandPalette.vue'
import ProfileSettingsView from './components/ProfileSettingsView.vue'
import SettingsView from './components/SettingsView.vue'
import FlowsView from './pipeline/components/FlowsView.vue'
import ActivityView from './components/ActivityView.vue'
import DeleteProfileModal from './components/DeleteProfileModal.vue'
import NewProfileModal from './components/NewProfileModal.vue'
import UnsavedFlowChangesModal from './components/UnsavedFlowChangesModal.vue'
import OnboardingScreen from './components/OnboardingScreen.vue'
import ToastStack from './components/ToastStack.vue'
import DevBar from './components/DevBar.vue'
import { useAuth } from './composables/useAuth'
import { useActivity } from './composables/useActivity'
import { useJobs } from './composables/useJobs'
import { useFeedState } from './composables/useFeedState'
import { useCommands, useCommandPalette, type Command } from './composables/useCommands'
import { comboFromEvent, formatCombo, useKeybindings } from './composables/useKeybindings'
import { commandCatalog } from './keybindings/catalog'
import { setTheme, themeLabels, themes } from './composables/useTheme'
import { useFlowsSession } from './pipeline/composables/useFlowsSession'
import { isEditableTarget } from './lib/isEditableTarget'
import { InstallUpdate, Status as UpdaterStatus } from '../bindings/github.com/colonyops/hive/desktop/updaterservice'
import type { UpdateInfo } from '../bindings/github.com/colonyops/hive/desktop/models'
import type { ApplicationSettingsSection, ProfileSettingsSection } from './router'
import type { InboxView, SidebarSelection } from './types/feed'
import { githubPayload } from './lib/feedPresentation'

// Only true when Vite is serving in dev mode (under `wails3 dev`); statically
// false in production/server builds, so DevBar is compiled out of them.
const devMode = import.meta.env.DEV

const {
  status: authStatus, authenticated, deviceFlow, card: authCard, error: authError, busy: authBusy,
  startDeviceFlow, useTokenInstead, backToStart, submitToken,
} = useAuth()

const {
  profiles, profilesLoaded, profilesError, activeProfile, activeProfileId, selection, items, visibleItems, unreadCount, search, loadError,
  selectedId, selectedItem, actions, pendingAction, actionRuns, sessionLaunchAction, sessionLaunchOptions, sessionLaunchBusy, sessionLaunchError, unreadOnly, title, toasts, dismissToast, clearToasts,
  creatingProfile, createProfileError, renamingProfile, renameProfileError, deletingProfile, loadProfiles, createProfile, renameProfile, deleteProfile,
  reorderFeeds, selectProfile, selectSidebar, selectUnreadView, selectItem, openActionRun, selectNext, selectPrev,
  toggleUnread, markItemUnread, toggleArchive, loadEvents, refresh, invokeAction, cancelSessionLaunch, submitSessionLaunch, notWired, openUrl, openSelectedInBrowser, hideWindow,
} = useFeedState()

// The feed-item kinds currently in the system — what the actions editor
// autocompletes and validates "applies to" against.
const knownFeedTypes = computed(() => [...new Set(items.value.map((item) => githubPayload(item).kind).filter(Boolean))].sort((a, b) => a.localeCompare(b)))


const selectedEvents = ref([] as Awaited<ReturnType<typeof loadEvents>>)
let selectedEventsSeq = 0
watch(selectedItem, async (item) => {
  const seq = ++selectedEventsSeq
  if (!item) {
    selectedEvents.value = []
    return
  }
  const events = await loadEvents(item.id)
  // A slower request for the previously selected item must never replace the
  // timeline for the item currently visible in the detail pane.
  if (seq === selectedEventsSeq && selectedItem.value?.id === item.id) selectedEvents.value = events
}, { immediate: true })

// ── Flows session (hc-8ft4yhm6) ──────────────────────────────────────────────
// A profile IS a flow, so the flows canvas is a per-profile sub-view: it swaps
// the sidebar+main region while the spaces rail and titlebar stay mounted (so
// the user is never stranded — see the template). Reached from the sidebar's
// "Flows" pill / "Edit flow" footer and the ⌘K command; exited by selecting a
// profile in the spaces rail or the ⌘K "Back to feed" command.
//
// The session (useFlowsSession) is a module singleton shared with
// FlowsView.vue: it owns the pipeline editor and a runtime for every enabled
// flow, so feeds keep updating while the canvas is closed or another profile
// is selected. App.vue is the first caller, which makes the manager app-lived
// rather than dependent on FlowsView mounting/unmounting.
const session = useFlowsSession()

// ── Route-driven navigation ────────────────────────────────────────────────
// The Wails webview uses hash history: routes survive asset:// hosting and
// native/browser back and forward controls traverse the same page stack. The
// shell (title bar + profile rail) stays mounted while this route selects its
// main page.
const router = useRouter()
const route = useRoute()
const flowsActive = computed(() => route.name === 'flows')
const activityActive = computed(() => route.name === 'activity')
const applicationSettingsActive = computed(() => route.name === 'application-settings')
const profileSettingsActive = computed(() => route.name === 'profile-settings')
const applicationSettingsSection = computed<ApplicationSettingsSection>(() =>
  route.params.section === 'integrations' ? 'integrations'
    : route.params.section === 'actions' ? 'actions'
      : route.params.section === 'keybindings' ? 'keybindings'
        : route.params.section === 'system' ? 'system'
          : 'appearance',
)
const profileSettingsSection = computed<ProfileSettingsSection>(() =>
  route.params.section === 'danger' ? 'danger' : 'general',
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
watch([profilesLoaded, () => route.fullPath], async ([loaded]) => {
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
  const feedId = typeof rawFeedId === 'string' && activeProfile.value?.feeds.some((feed) => feed.id === rawFeedId)
    ? rawFeedId
    : null
  const wantsUnread = route.query.unread === '1'
  const rawView = route.query.view
  const view: InboxView = typeof rawView === 'string' && ['open', 'archive', 'all', 'unfiled'].includes(rawView)
    ? rawView as InboxView
    : 'inbox'
  if (feedId) await selectSidebar({ type: 'feed', feedId })
  else if (wantsUnread) await selectUnreadView()
  else await selectSidebar({ type: 'view', view })

  // Unread can also filter one specific feed. selectSidebar clears the flag,
  // so apply it after loading that feed's items.
  if (feedId && wantsUnread && !unreadOnly.value) await toggleUnread()
}, { immediate: true })

watch([() => route.name, () => route.query.node], ([name, rawNode]) => {
  if (name === 'flows') session.openFlows(typeof rawNode === 'string' ? rawNode : undefined)
  else session.exitFlows()
}, { immediate: true })

// A router guard protects dirty flow drafts for every navigation source,
// including native mouse/browser Back — not only the app's own buttons.
type PendingNavigation = { to: string }
const pendingNavigation = ref<PendingNavigation | null>(null)
const unsavedChangesBusy = ref(false)
let allowGuardedNavigation = false
const removeNavigationGuard = router.beforeEach((to, from) => {
  const switchesProfile = typeof to.params.profileId === 'string' && to.params.profileId !== activeProfileId.value
  const leavesDirtyFlow = from.name === 'flows' && (
    to.name !== 'flows' || to.params.profileId !== from.params.profileId
  )
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
  const query = nextSelection.type === 'feed'
    ? { feed: nextSelection.feedId }
    : nextSelection.view === 'inbox' ? {} : { view: nextSelection.view }
  void router.push({ name: 'feed', params: { profileId: activeProfileId.value }, query })
}

function navigateUnreadFilter(value: boolean): void {
  if (!activeProfileId.value) return
  const query: Record<string, string> = {}
  if (selection.value.type === 'feed') query.feed = selection.value.feedId
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

function requestSelectProfile(id: string): void {
  openFeed(id)
}

function requestOpenActionsSettings(): void {
  void router.push({ name: 'application-settings', params: { section: 'actions' } })
}

function requestOpenSettings(page: 'application' | 'profile'): void {
  if (page === 'application') void router.push({ name: 'application-settings' })
  else if (activeProfileId.value) void router.push({ name: 'profile-settings', params: { profileId: activeProfileId.value } })
}

// ── Activity (6d) ─────────────────────────────────────────────────────────────
// App-global audit log. The titlebar's Activity link replaces the old "polling
// github" indicator; unseenActivity drives its dot.
const { unseenCount: unseenActivity } = useActivity()
const { activeJobs, hasActive: jobsActive } = useJobs()

function openActivity(): void {
  void router.push({ name: 'activity' })
}

async function openJobRun(commandID: number): Promise<void> {
  const job = activeJobs.value.find((candidate) => candidate.commandId === commandID)
  if (!job || !activeProfileId.value) return
  await router.push({ name: 'feed', params: { profileId: activeProfileId.value } })
  if (route.name !== 'feed') return
  await selectSidebar({ type: 'view', view: 'inbox' })
  await openActionRun(Number(job.target), job.actionId, commandID)
}

// ── Desktop self-update ───────────────────────────────────────────────────────
// The UpdaterService checks GitHub for a newer desktop release in the
// background (when enabled) and emits update:available. We seed initial state
// via Status() on mount and keep it current through the subscription, so the
// title-bar chip appears without waiting for the next poll. Clicking it
// downloads + relaunches into the new version.
const updateInfo = ref<UpdateInfo | null>(null)
const updateAvailable = computed(() => updateInfo.value?.available ?? false)
const updateLatestVersion = computed(() => updateInfo.value?.latestVersion ?? '')
const installingUpdate = ref(false)

async function openUpdate(): Promise<void> {
  if (installingUpdate.value) return
  // A native confirm is a lightweight affordance before the app quits and
  // relaunches into the new version; guarded for the non-Wails test context.
  if (typeof window.confirm === 'function' && !window.confirm('Download the update and relaunch Hive now?')) return
  installingUpdate.value = true
  try {
    await InstallUpdate()
  } catch (error) {
    console.debug('Update install failed', error)
    installingUpdate.value = false
  }
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

// ── Always-on runtime pump (hc-8ft4yhm6) ─────────────────────────────────────
// Drives every enabled runtime on each backend log append. The subscription
// lives here so processing continues with the canvas closed and regardless of
// profile selection. Commits complete BEFORE useFeedState.refresh() re-reads
// inbox items and membership claims, so all profile sidebars observe the newly committed work.
let unsubscribeLog: (() => void) | undefined
let unsubscribeFlowsRuntime: (() => void) | undefined
let unsubscribeUpdate: (() => void) | undefined
onMounted(() => {
  unsubscribeLog = Events.On('log:appended', () => {
    void (async () => {
      await session.pump()
      void refresh()
    })()
  })
  // The app owns this subscription, rather than FlowsView, because deployed
  // graphs must reload even while the canvas is closed. The session keeps an
  // unsaved editor draft private while replacing only its runtime snapshot.
  unsubscribeFlowsRuntime = Events.On('flows:updated', () => { void session.reloadDeployed() })
  // Seed the update chip from the last cached check, then react to background
  // checks. The event payload is the same UpdateInfo shape Status() returns.
  void UpdaterStatus().then((status) => { updateInfo.value = status }).catch((error) => {
    console.debug('Updater status unavailable', error)
  })
  unsubscribeUpdate = Events.On('update:available', (event: { data: UpdateInfo | UpdateInfo[] }) => {
    const payload = Array.isArray(event.data) ? event.data[0] : event.data
    if (payload) updateInfo.value = payload
  })
})
onUnmounted(() => {
  unsubscribeLog?.()
  unsubscribeFlowsRuntime?.()
  unsubscribeUpdate?.()
  session.disposeRuntime()
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

// Booting while signed out leaves profiles unloaded (or the live backend
// erroring); re-load the moment auth lands — and when the login changes, so
// a different account never sees the previous account's data.
watch(() => (authenticated.value ? authStatus.value?.login ?? '' : null), (key) => {
  if (key !== null) void loadProfiles()
})

// Step 2 of onboarding: authenticated but no workspace exists yet.
const needsWorkspace = computed(() => authenticated.value && profilesLoaded.value && profiles.value.length === 0)

// ── Layout chrome ─────────────────────────────────────────────────────────────
// The feed sidebar collapses to reclaim horizontal space (handy in split
// screens); the choice is persisted. Its toggle only appears in the feed view —
// the one place a sidebar renders — so it never dangles over settings or flows.
const sidebarCollapsed = useStorage('hive.panel.sidebar.collapsed', false)
const feedViewActive = computed(() =>
  authenticated.value && !needsWorkspace.value &&
  !applicationSettingsActive.value && !profileSettingsActive.value &&
  !flowsActive.value && !activityActive.value &&
  !!activeProfile.value,
)

function toggleSidebar(): void {
  sidebarCollapsed.value = !sidebarCollapsed.value
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

const { open: paletteOpen, toggle: togglePalette } = useCommandPalette()
const kb = useKeybindings()

// One handler per bindable command id. Both the keydown dispatcher and the
// command palette run through this map, so each command has a single
// implementation and the palette can show its live shortcut.
const runMap: Record<string, () => void | Promise<void>> = {
  'feed.next': selectNext,
  'feed.prev': selectPrev,
  'feed.open-in-browser': openSelectedInBrowser,
  'feed.toggle-unread': navigateUnreadToggle,
  'feed.refresh': refresh,
  'feed.toggle-archive': async () => { if (selectedItem.value) await toggleArchive(selectedItem.value) },
  'feed.mark-unread': async () => { if (selectedItem.value) await markItemUnread(selectedItem.value, true) },
  'palette.toggle': togglePalette,
  'window.hide': hideWindow,
}
const catalogById = new Map(commandCatalog.map((command) => [command.id, command]))

// The feed only accepts bare navigation keys when it is actually the on-screen
// view (matches the condition under which <FeedList> renders below).
const feedNavActive = computed(() =>
  route.name === 'feed' && authenticated.value && !needsWorkspace.value && !!activeProfile.value,
)

// While an overlay owns the screen, only the palette toggle stays live.
const anyOverlayOpen = computed(() =>
  paletteOpen.value || newProfileOpen.value || deleteProfileOpen.value || !!sessionLaunchAction.value || !!pendingNavigation.value,
)

// Seed commands — reactive getter so they update when profiles/flows load
useCommands(computed(() => {
  const cmds: Command[] = []

  // Bindable app commands (nav, refresh, …) with their live shortcut hint.
  for (const command of commandCatalog) {
    if (command.paletteHidden) continue
    cmds.push({
      id: command.id,
      title: command.title,
      group: command.group,
      keywords: command.keywords,
      icon: command.icon,
      hint: formatCombo(kb.bindings.value[command.id]?.[0] ?? ''),
      run: () => runMap[command.id]?.(),
    })
  }

  // Profiles
  for (const p of profiles.value) {
    cmds.push({
      id: `profile:${p.id}`,
      title: `Switch to profile: ${p.name}`,
      group: 'Profiles',
      icon: IconLayoutGrid,
      run: () => requestSelectProfile(p.id),
    })
  }

  // Feeds — All items always first, then individual feeds
  const profileName = activeProfile.value?.name

  cmds.push({
    id: 'feed:all',
    title: 'Select feed: All items',
    group: 'Feeds',
    icon: IconList,
    hint: profileName,
    run: () => navigateSidebar({ type: 'view', view: 'inbox' }),
  })

  for (const f of activeProfile.value?.feeds ?? []) {
    cmds.push({
      id: `feed:${f.id}`,
      title: `Select feed: ${f.name}`,
      group: 'Feeds',
      icon: IconRss,
      hint: profileName,
      run: () => navigateSidebar({ type: 'feed', feedId: f.id }),
    })
  }

  cmds.push({
    id: 'profile:new',
    title: 'New profile…',
    group: 'Profiles',
    keywords: ['workspace', 'create'],
    run: openNewProfile,
  })

  // View — enter/exit the flows canvas for the active profile.
  cmds.push({
    id: 'flow:edit',
    title: flowsActive.value ? 'Back to feed' : 'Edit flow…',
    group: 'View',
    keywords: ['flows', 'pipeline', 'nodes', 'canvas', 'editor'],
    icon: IconWorkflow,
    run: () => { flowsActive.value ? requestExitFlows() : openFlows() },
  })

  // Jump to any node in the active flow by name (8d) — opens the canvas
  // focused/centered on that node, same as "Reveal in flow" from the sidebar.
  for (const node of session.activeFlow.value?.nodes ?? []) {
    cmds.push({
      id: `flow:node:${node.id}`,
      title: `Jump to node: ${node.name || node.type}`,
      group: 'Flow',
      keywords: ['flows', 'node', 'canvas', 'reveal'],
      icon: IconShare2,
      run: () => openFlows(node.id),
    })
  }

  // Themes
  for (const t of themes) {
    cmds.push({
      id: `theme:${t}`,
      title: `Theme: ${themeLabels[t]}`,
      group: 'Theme',
      keywords: ['theme', 'appearance', t],
      icon: IconPalette,
      run: () => setTheme(t),
    })
  }

  return cmds
}))

// ── Global input navigation ──────────────────────────────────────────────────
// Resolves a keydown against the configurable keymap and runs the matched
// command. Bare (modifier-less) keys are ignored while typing; feed commands
// only fire on the feed; overlays suppress everything but the palette toggle.

function onGlobalKeydown(e: KeyboardEvent): void {
  // WebKit can treat an unhandled Backspace as browser Back. Suppress that
  // default outside editors while still allowing components such as the flow
  // canvas to use Backspace for their own actions.
  if (e.key === 'Backspace' && !isEditableTarget(e.target)) e.preventDefault()

  if (kb.recording.value) return // the settings editor is capturing this key
  const combo = comboFromEvent(e)
  if (!combo) return
  const id = kb.resolve(combo)
  if (!id) return
  const command = catalogById.get(id)
  if (!command) return

  const mods = combo.split('+')
  const hasModifier = mods.includes('mod') || mods.includes('ctrl') || mods.includes('alt')
  if (isEditableTarget(e.target) && !hasModifier) return

  if (anyOverlayOpen.value && id !== 'palette.toggle') return
  if (command.context === 'feed' && !feedNavActive.value) return

  e.preventDefault()
  void runMap[id]?.()
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

onMounted(() => {
  window.addEventListener('keydown', onGlobalKeydown)
  window.addEventListener('mousedown', preventNativeMouseHistory)
  window.addEventListener('mouseup', onGlobalMouseUp)
  window.addEventListener('auxclick', preventNativeMouseHistory)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onGlobalKeydown)
  window.removeEventListener('mousedown', preventNativeMouseHistory)
  window.removeEventListener('mouseup', onGlobalMouseUp)
  window.removeEventListener('auxclick', preventNativeMouseHistory)
})
</script>

<template>
  <main class="h-screen w-screen overflow-hidden bg-app text-text">
    <div class="flex h-full min-h-0 flex-col overflow-hidden">
      <TitleBar
        :profile-name="authenticated && !needsWorkspace ? activeProfile?.name ?? 'Loading' : undefined"
        :activity-active="activityActive"
        :error-count="errorCount"
        :unseen-activity="unseenActivity"
        :jobs-active="jobsActive"
        :active-jobs="activeJobs"
        :update-available="updateAvailable"
        :latest-version="updateLatestVersion"
        :can-go-back="canGoBack"
        :can-go-forward="canGoForward"
        :sidebar-collapsed="sidebarCollapsed"
        :can-toggle-sidebar="feedViewActive"
        @back="router.back()"
        @forward="router.forward()"
        @open-error-node="openErrorNode"
        @open-activity="openActivity"
        @open-job-run="openJobRun"
        @open-update="openUpdate"
        @toggle-sidebar="toggleSidebar"
        @open-palette="togglePalette"
        @toggle-maximise="toggleMaximise"
      />
      <!-- Hold an empty frame until auth status resolves so an authenticated
           user never sees onboarding flash by. -->
      <div v-if="authStatus === null" class="flex min-h-0 flex-1 items-center justify-center font-mono text-xs text-text-4">Loading…</div>
      <OnboardingScreen
        v-else-if="!authenticated || needsWorkspace"
        :card="needsWorkspace ? 'workspace' : authCard"
        :device-flow="deviceFlow"
        :error="needsWorkspace ? createProfileError : authError"
        :busy="needsWorkspace ? creatingProfile : authBusy"
        @start-device-flow="startDeviceFlow"
        @use-token-instead="useTokenInstead"
        @back-to-start="backToStart"
        @submit-token="submitToken"
        @create-workspace="createProfile"
      />
      <!-- The spaces rail (ProfileRail) and TitleBar stay mounted across the
           feed<->flows switch; only the sidebar+main region swaps. This is
           what keeps the user from being stranded in the flows canvas — the
           spaces rail is always there to navigate back. -->
      <div v-else class="flex min-h-0 flex-1">
        <ProfileRail
          :profiles="profiles"
          :active-profile-id="activeProfileId"
          @select="requestSelectProfile"
          @add="openNewProfile"
          @open-settings="requestOpenSettings('application')"
        />
        <SettingsView
          v-if="applicationSettingsActive"
          :github-connected="authenticated"
          :github-login="authStatus?.login"
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
          @close="closeSettings"
          @rename="submitProfileRename"
          @delete="openDeleteProfile"
          @select-section="selectProfileSettingsSection"
        />
        <FlowsView v-else-if="flowsActive" />
        <ActivityView v-else-if="activityActive" @close="closeSettings" />
        <template v-else>
          <SideBar
            v-if="activeProfile && !sidebarCollapsed"
            :profile="activeProfile"
            :selection="selection"
            :flows-dirty="session.dirty.value"
            @select="navigateSidebar"
            @open-flows="openFlows()"
            @open-settings="requestOpenSettings('profile')"
            @reorder="(t) => activeProfile && reorderFeeds(activeProfile.id, t)"
          />
          <section v-if="activeProfile" class="flex min-w-0 flex-1">
            <FeedList
              :title="title"
              :visible-items="visibleItems"
              :selected-id="selectedId"
              :unread-only="unreadOnly"
              :unread-count="unreadCount"
              :view="selection.type === 'view' ? selection.view : 'inbox'"
              :search="search"
              :load-error="loadError"
              @select="selectItem"
              @update:search="(value) => (search = value)"
              @set-unread="navigateUnreadFilter"
              @refresh="refresh"
            />
            <DetailPane :item="selectedItem" :events="selectedEvents" :actions="actions" :pending-action="pendingAction" :action-runs="actionRuns" @run-action="invokeAction" @open-browser="openSelectedInBrowser" @open-url="openUrl" @edit="requestOpenActionsSettings" />
          </section>
          <div v-else class="flex flex-1 flex-col items-center justify-center gap-3 font-mono text-xs text-text-4">
            <template v-if="profilesError">
              <span data-testid="profiles-error">{{ profilesError }}</span>
              <button class="cursor-pointer rounded border border-strong px-3 py-1.5 text-text-2 hover:text-text" @click="loadProfiles">Retry</button>
            </template>
            <span v-else>Loading feed…</span>
          </div>
        </template>
      </div>
      <DevBar v-if="devMode" />
    </div>
    <CreateSessionDialog
      v-if="sessionLaunchAction && sessionLaunchOptions"
      :action-label="sessionLaunchAction.label"
      :options="sessionLaunchOptions"
      :busy="sessionLaunchBusy"
      :error="sessionLaunchError"
      @close="cancelSessionLaunch"
      @submit="submitSessionLaunch"
    />
    <ToastStack :toasts="toasts" @dismiss="dismissToast" @clear-all="clearToasts" />
    <CommandPalette />
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
    <UnsavedFlowChangesModal
      v-if="pendingNavigation"
      :busy="unsavedChangesBusy"
      :error="session.error.value"
      @close="cancelPendingNavigation"
      @deploy="deployPendingNavigation"
      @discard="discardPendingNavigation"
    />
  </main>
</template>
