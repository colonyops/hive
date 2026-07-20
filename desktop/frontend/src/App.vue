<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import IconEye from '~icons/lucide/eye'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import IconList from '~icons/lucide/list'
import IconMinus from '~icons/lucide/minus'
import IconPalette from '~icons/lucide/palette'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconRss from '~icons/lucide/rss'
import IconShare2 from '~icons/lucide/share-2'
import IconWorkflow from '~icons/lucide/workflow'
import TitleBar from './components/TitleBar.vue'
import ProfileRail from './components/ProfileRail.vue'
import SideBar from './components/SideBar.vue'
import FeedList from './components/FeedList.vue'
import DetailPane from './components/DetailPane.vue'
import CommandPalette from './components/CommandPalette.vue'
import SettingsView from './components/SettingsView.vue'
import FlowsView from './pipeline/components/FlowsView.vue'
import DeleteProfileModal from './components/DeleteProfileModal.vue'
import NewProfileModal from './components/NewProfileModal.vue'
import UnsavedFlowChangesModal from './components/UnsavedFlowChangesModal.vue'
import OnboardingScreen from './components/OnboardingScreen.vue'
import ToastStack from './components/ToastStack.vue'
import { useAuth } from './composables/useAuth'
import { useFeedState } from './composables/useFeedState'
import { useCommands, useCommandPalette, type Command } from './composables/useCommands'
import { setTheme, themeLabels, themes } from './composables/useTheme'
import { useFlowsSession } from './pipeline/composables/useFlowsSession'

const {
  status: authStatus, authenticated, deviceFlow, card: authCard, error: authError, busy: authBusy,
  startDeviceFlow, useTokenInstead, backToStart, submitToken,
} = useAuth()

const {
  profiles, profilesLoaded, profilesError, activeProfile, activeProfileId, selection, items, loadError,
  selectedId, selectedItem, actions, unreadOnly, title, countLabel, toasts, dismissToast, clearToasts,
  creatingProfile, createProfileError, deletingProfile, loadProfiles, createProfile, deleteProfile,
  selectProfile, selectSidebar, selectUnreadView, selectItem,
  toggleUnread, refresh, notWired, openUrl, openSelectedInBrowser, hideWindow,
} = useFeedState()

// ── Flows session (hc-8ft4yhm6) ──────────────────────────────────────────────
// A profile IS a flow, so the flows canvas is a per-profile sub-view: it swaps
// the sidebar+main region while the spaces rail and titlebar stay mounted (so
// the user is never stranded — see the template). Reached from the sidebar's
// "Flows" pill / "Edit flow" footer and the ⌘K command; exited via the
// titlebar breadcrumb.
//
// The session (useFlowsSession) is a module singleton shared with
// FlowsView.vue: it owns the pipeline editor AND the always-on runtime that
// commits feed_item for the active profile's flow, so the sidebar populates
// even while the canvas below is never opened. App.vue is the FIRST caller
// (this line runs during App's own setup), so the session's internal
// onMounted/watch hooks bind to App's lifetime — it keeps running for as
// long as the app does, independent of FlowsView mounting/unmounting.
const session = useFlowsSession()

// Bind the session's tracked flow to whichever profile is active, and
// switching profiles from the flows canvas returns to the feed view of the
// new profile — the just-opened flow belonged to the previous profile.
watch(activeProfileId, (id) => {
  session.bindActiveFlow(id || undefined)
  session.exitFlows()
})

// ── Un-deployed changes guard (hc-sx4k3c7k) ──────────────────────────────
// Exiting the canvas and switching the active profile both silently
// abandon un-deployed flow edits (session.dirty) today. Both call sites
// route through the request*() wrappers below instead of touching
// flowsOpen/activeProfileId directly, so when dirty the actual navigation
// is deferred behind a confirm modal (Deploy/Discard/Cancel) rather than
// firing immediately — nothing has changed yet when the modal opens, so
// Cancel needs no "undo": the rail selection and the real active profile
// were never touched in the first place.
type PendingNavigation = { kind: 'exit-flows' } | { kind: 'switch-profile'; profileId: string }
const pendingNavigation = ref<PendingNavigation | null>(null)
const unsavedChangesBusy = ref(false)

function requestExitFlows(): void {
  if (session.dirty.value) pendingNavigation.value = { kind: 'exit-flows' }
  else session.exitFlows()
}

function requestSelectProfile(id: string): void {
  if (session.dirty.value && id !== activeProfileId.value) pendingNavigation.value = { kind: 'switch-profile', profileId: id }
  else void selectProfile(id)
}

function applyPendingNavigation(pending: PendingNavigation): void {
  if (pending.kind === 'exit-flows') session.exitFlows()
  else void selectProfile(pending.profileId)
}

function cancelPendingNavigation(): void {
  pendingNavigation.value = null
}

async function deployPendingNavigation(): Promise<void> {
  const pending = pendingNavigation.value
  if (!pending) return
  unsavedChangesBusy.value = true
  await session.deploy()
  unsavedChangesBusy.value = false
  if (session.dirty.value) return // deploy failed — session.error carries the message; stay open so the user can retry or cancel
  pendingNavigation.value = null
  applyPendingNavigation(pending)
}

async function discardPendingNavigation(): Promise<void> {
  const pending = pendingNavigation.value
  if (!pending) return
  unsavedChangesBusy.value = true
  await session.discardDraft()
  unsavedChangesBusy.value = false
  if (session.dirty.value) return // reload from disk failed — stay open
  pendingNavigation.value = null
  applyPendingNavigation(pending)
}

// ── Settings view (hc-pppw2iww) ──────────────────────────────────────────────
// A plain app-level view-state ref — NOT folded into useFlowsSession, since
// settings is an app-chrome concern independent of any profile/flow. Mirrors
// how session.flowsOpen swaps the main region (see the template): SideBar's
// "open-settings" sets this true and renders SettingsView full-screen in
// place of the sidebar+feed; its own header "Back to feed" button (or Esc)
// sets it back to false. Opening settings also exits the flows canvas so the
// two full-screen views never stack — safe to do unconditionally (unlike
// requestExitFlows) because SideBar (the only place the settings button
// lives today) is itself only rendered once flows is already closed.
const settingsOpen = ref(false)

function openSettings(): void {
  settingsOpen.value = true
  session.exitFlows()
}

function closeSettings(): void {
  settingsOpen.value = false
}

// ── Reveal in flow (8d) ───────────────────────────────────────────────────────
// A sidebar feed row's id is flow-qualified ("<activeProfileId>/<nodeId>" —
// see useFeedState's loadFeeds), so the node id is everything after the
// profile id's own "/" separator. openFlows() both opens the canvas and sets
// flowFocusNodeId, which FlowsCanvas's existing focus watch turns into a
// select + center-pan (see FlowsView.vue's :focus-node-id binding).
function revealInFlow(feedId: string): void {
  const nodeId = feedId.slice(activeProfileId.value.length + 1)
  session.openFlows(nodeId)
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
  if (firstErrorNodeId.value) session.openFlows(firstErrorNodeId.value)
}

// ── Always-on runtime pump (hc-8ft4yhm6) ─────────────────────────────────────
// Drives the shared session's runtime on every backend log append —
// mirrors FlowsView's old per-canvas subscription (see usePipelineRuntime's
// module docs for why the mounting component owns Events.On rather than the
// composable), except this one lives here so it keeps pumping with the
// canvas closed. The commit must complete BEFORE useFeedState.refresh()
// re-reads feed_item, or the sidebar would race the write and show stale
// counts/items — hence the `await` ahead of the (fire-and-forget) refresh.
let unsubscribeLog: (() => void) | undefined
onMounted(() => {
  unsubscribeLog = Events.On('log:appended', () => {
    void (async () => {
      await session.pump()
      void refresh()
    })()
  })
})
onUnmounted(() => { unsubscribeLog?.() })

// ── Profile create / delete overlays ─────────────────────────────────────────

const newProfileOpen = ref(false)

function openNewProfile() {
  createProfileError.value = null // a stale failure must not greet the reopen
  newProfileOpen.value = true
}

async function submitNewProfile(name: string) {
  await createProfile(name)
  if (!createProfileError.value) newProfileOpen.value = false
}

const deleteProfileOpen = ref(false)

function openDeleteProfile() {
  deleteProfileOpen.value = true
}

async function confirmDeleteProfile() {
  if (!activeProfileId.value) return
  await deleteProfile(activeProfileId.value)
  deleteProfileOpen.value = false
}

// Booting while signed out leaves profiles unloaded (or the live backend
// erroring); re-load the moment auth lands — and when the login changes, so
// a different account never sees the previous account's data.
watch(() => (authenticated.value ? authStatus.value?.login ?? '' : null), (key) => {
  if (key !== null) void loadProfiles()
})

// Step 2 of onboarding: authenticated but no workspace exists yet.
const needsWorkspace = computed(() => authenticated.value && profilesLoaded.value && profiles.value.length === 0)

// ── Command palette ──────────────────────────────────────────────────────────

const { open: paletteOpen, toggle: togglePalette } = useCommandPalette()

// Seed commands — reactive getter so they update when profiles/feeds load
useCommands(computed(() => {
  const cmds: Command[] = []

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
    run: () => selectSidebar({ type: 'all' }),
  })

  for (const f of activeProfile.value?.feeds ?? []) {
    cmds.push({
      id: `feed:${f.id}`,
      title: `Select feed: ${f.name}`,
      group: 'Feeds',
      icon: IconRss,
      hint: profileName,
      run: () => selectSidebar({ type: 'feed', feedId: f.id }),
    })
  }

  cmds.push({
    id: 'feed:toggle-unread',
    title: 'Toggle unread filter',
    group: 'Feeds',
    icon: IconEye,
    run: toggleUnread,
  })

  cmds.push({
    id: 'feed:refresh',
    title: 'Refresh feeds',
    group: 'Feeds',
    keywords: ['reload', 'sync'],
    icon: IconRefreshCw,
    run: refresh,
  })

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
    title: session.flowsOpen.value ? 'Back to feed' : 'Edit flow…',
    group: 'View',
    keywords: ['flows', 'pipeline', 'nodes', 'canvas', 'editor'],
    icon: IconWorkflow,
    run: () => { session.flowsOpen.value ? requestExitFlows() : session.openFlows() },
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
      run: () => session.openFlows(node.id),
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

  // Window
  cmds.push({
    id: 'window:hide',
    title: 'Hide window',
    group: 'Window',
    icon: IconMinus,
    run: hideWindow,
  })

  return cmds
}))

// ── Global ⌘K / Ctrl+K listener ──────────────────────────────────────────────

function onGlobalKeydown(e: KeyboardEvent): void {
  if (e.key.toLowerCase() !== 'k' || (!e.metaKey && !e.ctrlKey)) return

  // If palette is already open, always close it (even from inside the input)
  if (paletteOpen.value) {
    e.preventDefault()
    togglePalette()
    return
  }

  // Ignore ⌘K while focus is in any editable element other than the palette input
  const target = e.target as HTMLElement
  const isEditable =
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target.isContentEditable
  if (isEditable) return

  e.preventDefault()
  togglePalette()
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onUnmounted(() => window.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
  <main class="h-screen w-screen overflow-hidden bg-app text-text">
    <div class="flex h-full min-h-0 flex-col overflow-hidden">
      <TitleBar
        :profile-name="authenticated && !needsWorkspace ? activeProfile?.name ?? 'Loading' : undefined"
        :unread-count="activeProfile?.unreadCount ?? 0"
        :flows-active="session.flowsOpen.value"
        :error-count="errorCount"
        @exit-flows="requestExitFlows"
        @open-error-node="openErrorNode"
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
           rail and the breadcrumb are always there to navigate back. -->
      <div v-else class="flex min-h-0 flex-1">
        <ProfileRail :profiles="profiles" :active-profile-id="activeProfileId" @select="requestSelectProfile" @add="openNewProfile" />
        <SettingsView v-if="settingsOpen" @close="closeSettings" />
        <FlowsView v-else-if="session.flowsOpen.value" />
        <template v-else>
          <SideBar
            v-if="activeProfile"
            :profile="activeProfile"
            :selection="selection"
            :unread-only="unreadOnly"
            :flows-dirty="session.dirty.value"
            @select="selectSidebar"
            @select-unread="selectUnreadView"
            @delete-profile="openDeleteProfile"
            @open-flows="session.openFlows()"
            @open-settings="openSettings"
            @reveal-in-flow="revealInFlow"
          />
          <section v-if="activeProfile" class="flex min-w-0 flex-1">
            <FeedList
              :title="title"
              :items="items"
              :selected-id="selectedId"
              :unread-only="unreadOnly"
              :count-label="countLabel"
              :load-error="loadError"
              @select="selectItem"
              @toggle-unread="toggleUnread"
              @refresh="refresh"
            />
            <DetailPane :item="selectedItem" :actions="actions" @run-action="notWired" @open-browser="openSelectedInBrowser" @open-url="openUrl" @edit="notWired" />
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
    </div>
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
