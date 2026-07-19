<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import IconEye from '~icons/lucide/eye'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import IconList from '~icons/lucide/list'
import IconMinus from '~icons/lucide/minus'
import IconPalette from '~icons/lucide/palette'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconRss from '~icons/lucide/rss'
import IconWorkflow from '~icons/lucide/workflow'
import TitleBar from './components/TitleBar.vue'
import ProfileRail from './components/ProfileRail.vue'
import SideBar from './components/SideBar.vue'
import FeedList from './components/FeedList.vue'
import DetailPane from './components/DetailPane.vue'
import CommandPalette from './components/CommandPalette.vue'
import FlowsView from './pipeline/components/FlowsView.vue'
import ConfigErrorOverlay from './components/ConfigErrorOverlay.vue'
import ConfigSheet from './components/ConfigSheet.vue'
import FeedEditorSheet from './components/FeedEditorSheet.vue'
import DeleteProfileModal from './components/DeleteProfileModal.vue'
import NewProfileModal from './components/NewProfileModal.vue'
import OnboardingScreen from './components/OnboardingScreen.vue'
import ToastStack from './components/ToastStack.vue'
import { useAuth } from './composables/useAuth'
import { useFeedState } from './composables/useFeedState'
import { useCommands, useCommandPalette, type Command } from './composables/useCommands'
import { setTheme, themeLabels, themes } from './composables/useTheme'
import { parseConfigErrors } from './lib/configErrors'
import type { FeedDef, SourceDef } from './types/feed'

const {
  status: authStatus, authenticated, deviceFlow, card: authCard, error: authError, busy: authBusy,
  startDeviceFlow, useTokenInstead, backToStart, submitToken,
} = useAuth()

const {
  profiles, profilesLoaded, profilesError, activeProfile, activeProfileId, selection, items, loadError,
  selectedId, selectedItem, actions, unreadOnly, title, countLabel, toasts, dismissToast, clearToasts,
  creatingProfile, createProfileError, deletingProfile, loadProfiles, createProfile, deleteProfile,
  config, configErrorOverlayOpen, dismissConfigError, loadConfig, copyConfigPrompt, copyConfigPath, copyConfigErrors,
  sources, creatingSource, createSourceError, savingFeed, saveFeedError,
  loadSources, createSource, loadFeedDef, createFeed, updateFeed, deleteFeed,
  selectProfile, selectSidebar, selectUnreadView, selectItem,
  toggleUnread, refresh, notWired, hideWindow,
} = useFeedState()

// The overlay only ever gets the shape the backend actually provides today
// (see internal/desktop/feed/config.go ConfigInfo.Error — a single string);
// parseConfigErrors is the seam where richer per-problem data would plug in
// without touching the template.
const configErrors = computed(() => parseConfigErrors(config.value?.error ?? ''))

// ── Flows editor mode ────────────────────────────────────────────────────────
// A profile IS a flow, so the flows canvas is a per-profile sub-view: it swaps
// the sidebar+main region while the spaces rail and titlebar stay mounted (so
// the user is never stranded — see the template). Reached from the sidebar's
// "Flows" pill / "Edit flow" footer and the ⌘K command; exited via the
// titlebar breadcrumb.
const mode = ref<'feed' | 'flows'>('feed')

function openFlows() {
  mode.value = 'flows'
}

function exitFlows() {
  mode.value = 'feed'
}

// Switching profiles from the flows canvas returns to the feed view of the new
// profile — the just-opened flow belonged to the previous profile.
watch(activeProfileId, () => { mode.value = 'feed' })

// ── Config sheet & profile creation overlays ─────────────────────────────────

const configSheetOpen = ref(false)
const newProfileOpen = ref(false)

function openConfigSheet() {
  configSheetOpen.value = true
  void loadConfig()
}

function openNewProfile() {
  createProfileError.value = null // a stale failure must not greet the reopen
  newProfileOpen.value = true
}

async function submitNewProfile(name: string) {
  await createProfile(name)
  if (!createProfileError.value) newProfileOpen.value = false
}

// ── Feed editor sheet ────────────────────────────────────────────────────────

const feedEditorOpen = ref(false)
// Non-null while editing an existing feed; null in create mode.
const editingFeedId = ref<string | null>(null)
// The edit-mode prefill, loaded async after the sheet opens.
const editingFeedDef = ref<FeedDef | null>(null)

function openFeedEditor(feedId?: string) {
  editingFeedId.value = feedId ?? null
  editingFeedDef.value = null
  createSourceError.value = null // stale failures must not greet the reopen
  saveFeedError.value = null
  feedEditorOpen.value = true
  void loadSources()
  void loadConfig() // the sheet shows the config path row
  if (feedId && activeProfileId.value) {
    void loadFeedDef(activeProfileId.value, feedId).then((def) => { editingFeedDef.value = def })
  }
}

async function submitFeedSave(def: FeedDef) {
  if (!activeProfileId.value) return
  const saved = editingFeedId.value
    ? await updateFeed(activeProfileId.value, editingFeedId.value, def)
    : await createFeed(activeProfileId.value, def)
  if (saved) feedEditorOpen.value = false
}

async function submitFeedDelete(feedId: string) {
  if (!activeProfileId.value) return
  const deleted = await deleteFeed(activeProfileId.value, feedId)
  if (deleted) feedEditorOpen.value = false
}

function submitNewSource(def: SourceDef) {
  void createSource(def)
}

// ── Delete profile confirm modal ─────────────────────────────────────────────

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
      run: () => selectProfile(p.id),
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
    cmds.push({
      id: `feed:edit:${f.id}`,
      title: `Edit feed: ${f.name}`,
      group: 'Feeds',
      keywords: ['editor', 'filters'],
      run: () => openFeedEditor(f.id),
    })
  }

  cmds.push({
    id: 'feed:new',
    title: 'New feed…',
    group: 'Feeds',
    keywords: ['create', 'editor', 'source'],
    run: () => openFeedEditor(),
  })

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

  cmds.push({
    id: 'feed:edit-config',
    title: 'Edit feeds as code…',
    group: 'Feeds',
    keywords: ['config', 'yaml', 'profiles'],
    run: openConfigSheet,
  })

  cmds.push({
    id: 'feed:copy-config-prompt',
    title: 'Copy feeds config prompt',
    group: 'Feeds',
    keywords: ['config', 'yaml', 'agent', 'prompt'],
    run: copyConfigPrompt,
  })

  // View
  cmds.push({
    id: 'view:toggle-flows',
    title: mode.value === 'feed' ? 'Open flows editor…' : 'Back to feed',
    group: 'View',
    keywords: ['flows', 'pipeline', 'nodes', 'canvas', 'editor'],
    icon: IconWorkflow,
    run: () => { mode.value = mode.value === 'feed' ? 'flows' : 'feed' },
  })

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
  <main
    class="h-screen w-screen overflow-hidden bg-app text-text"
    :class="{ 'pointer-events-none blur-[3px] opacity-40 transition-[filter,opacity] duration-200': configErrorOverlayOpen }"
  >
    <div class="flex h-full min-h-0 flex-col overflow-hidden">
      <TitleBar
        :profile-name="authenticated && !needsWorkspace ? activeProfile?.name ?? 'Loading' : undefined"
        :unread-count="activeProfile?.unreadCount ?? 0"
        :flows-active="mode === 'flows'"
        @exit-flows="exitFlows"
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
        <ProfileRail :profiles="profiles" :active-profile-id="activeProfileId" @select="selectProfile" @add="openNewProfile" />
        <FlowsView v-if="mode === 'flows'" :flow-id="activeProfileId" />
        <template v-else>
          <SideBar
            v-if="activeProfile"
            :profile="activeProfile"
            :selection="selection"
            :unread-only="unreadOnly"
            @select="selectSidebar"
            @select-unread="selectUnreadView"
            @edit-feeds="openConfigSheet"
            @edit-feed="openFeedEditor"
            @delete-profile="openDeleteProfile"
            @open-flows="openFlows"
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
            <DetailPane :item="selectedItem" :actions="actions" @run-action="notWired" @open-browser="notWired" @edit="notWired" />
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
    <ConfigSheet
      v-if="configSheetOpen"
      :config="config"
      @close="configSheetOpen = false"
      @copy-prompt="copyConfigPrompt"
      @copy-path="copyConfigPath"
    />
    <FeedEditorSheet
      v-if="feedEditorOpen"
      :feed-id="editingFeedId"
      :initial-def="editingFeedDef"
      :sources="sources"
      :config="config"
      :busy="savingFeed"
      :error="saveFeedError"
      :source-busy="creatingSource"
      :source-error="createSourceError"
      @close="feedEditorOpen = false"
      @save="submitFeedSave"
      @delete="submitFeedDelete"
      @create-source="submitNewSource"
      @copy-prompt="copyConfigPrompt"
      @copy-path="copyConfigPath"
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
  </main>
  <ConfigErrorOverlay
    v-if="configErrorOverlayOpen"
    :path="config?.path ?? ''"
    :errors="configErrors"
    @retry="loadConfig"
    @dismiss="dismissConfigError"
    @copy-path="copyConfigPath"
    @copy-errors="copyConfigErrors"
  />
</template>
