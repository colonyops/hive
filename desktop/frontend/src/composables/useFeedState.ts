import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Events, Window } from '@wailsio/runtime'
import { ActionsFor, Config, ConfigPrompt, CreateFeed, CreateProfile, CreateSource, DeleteFeed, DeleteProfile, FeedDefFor, Items, MarkRead, Profiles, Refresh, Sources, UpdateFeed } from '../../bindings/github.com/colonyops/hive/desktop/feedservice'
import type { Action, ConfigInfo, FeedDef, FeedItem, Profile, SidebarSelection, SourceDef } from '../types/feed'
import type { ToastInstance, ToastOptions } from '../types/toast'

export function useFeedState() {
  const profiles = ref<Profile[]>([])
  // False until the first Profiles() resolves: the app must distinguish
  // "no workspaces yet" (onboarding step 2) from "not loaded yet".
  const profilesLoaded = ref(false)
  // Set when loadProfiles fails; distinct from "loaded, empty" (see above).
  const profilesError = ref<string | null>(null)
  const activeProfileId = ref('')
  // Scope (all | one feed) and the unread filter are independent axes; the
  // sidebar "Unread" view is all-scope with the filter on.
  const selection = ref<SidebarSelection>({ type: 'all' })
  const unreadOnly = ref(false)
  const items = ref<FeedItem[]>([])
  // User-facing description of the last failed load; null when healthy.
  const loadError = ref<string | null>(null)
  const selectedId = ref<string | null>(null)
  const actions = ref<Action[]>([])
  // Stacked toasts (design spec "6a Toasts"): bottom-right, up to ~4 visible,
  // each with its own auto-dismiss timer. Error-severity toasts never
  // auto-dismiss — they sit until the user clears them or resolves the
  // underlying failure.
  const toasts = ref<ToastInstance[]>([])
  const creatingProfile = ref(false)
  const createProfileError = ref<string | null>(null)
  const deletingProfile = ref(false)
  // Top-level source definitions for the feed editor's picker; loaded when
  // the editor opens.
  const sources = ref<SourceDef[]>([])
  const creatingSource = ref(false)
  const createSourceError = ref<string | null>(null)
  // One busy/error pair covers create and update: the editor sheet only ever
  // runs one save at a time.
  const savingFeed = ref(false)
  const saveFeedError = ref<string | null>(null)
  // The profiles config file (path, content, validity) for the
  // feeds-as-code sheet; null until first loaded.
  const config = ref<ConfigInfo | null>(null)
  // The config-error overlay (design spec "6b") is dismissible: the user can
  // keep working on the last-good config. Dismissal is cleared whenever a
  // *new* invalid state arrives (fresh error text, or a valid→invalid
  // transition) so an unresolved edit that fails again still interrupts.
  const configErrorDismissed = ref(false)
  let nextToastId = 1
  const toastTimers = new Map<number, ReturnType<typeof setTimeout>>()
  const defaultToastDuration = 4000
  // Monotonic token: loadItems responses arriving out of order (slow feed
  // switch, live backend latency) must not clobber the newest request.
  let loadSeq = 0
  // Re-entry guard for deleteFeed: the sheet's confirm button collapses back
  // to the plain "Delete feed" button synchronously on emit, so a fast
  // second click could otherwise fire a second delete mid-flight.
  let deletingFeed = false

  const activeProfile = computed(() => profiles.value.find((profile) => profile.id === activeProfileId.value) ?? null)
  const selectedItem = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
  const title = computed(() => {
    const currentSelection = selection.value
    if (currentSelection.type === 'feed') return activeProfile.value?.feeds?.find((feed) => feed.id === currentSelection.feedId)?.name ?? 'Feed'
    return unreadOnly.value ? 'Unread' : 'All items'
  })
  const countLabel = computed(() => activeProfile.value ? `${activeProfile.value.totalCount} · ${activeProfile.value.unreadCount} unread` : '')
  // Full-app blocking overlay (design spec "6b"): shown whenever the loaded
  // config is invalid and the user hasn't dismissed *this* error.
  const configErrorOverlayOpen = computed(() => !!config.value && !config.value.valid && !configErrorDismissed.value)

  function showToast(message: string, options: ToastOptions = {}): number {
    const severity = options.severity ?? 'info'
    const id = nextToastId++
    // Error toasts persist until manually cleared or resolved — auto-dismiss
    // would hide a failure the user hasn't necessarily seen yet.
    const duration = severity === 'error' ? null : options.duration ?? defaultToastDuration
    toasts.value = [...toasts.value, { id, message, body: options.body, severity, actions: options.actions ?? [], duration }]
    if (duration !== null) {
      toastTimers.set(id, setTimeout(() => dismissToast(id), duration))
    }
    return id
  }

  function dismissToast(id: number) {
    const timer = toastTimers.get(id)
    if (timer !== undefined) {
      clearTimeout(timer)
      toastTimers.delete(id)
    }
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  function clearToasts() {
    for (const timer of toastTimers.values()) clearTimeout(timer)
    toastTimers.clear()
    toasts.value = []
  }

  function dismissConfigError() {
    configErrorDismissed.value = true
  }

  async function loadActions(item: FeedItem | null) {
    if (!item) {
      actions.value = []
      return
    }
    try {
      actions.value = (await ActionsFor(item.kind)) ?? []
    } catch (error) {
      console.warn('Unable to load actions', error)
      actions.value = []
    }
  }

  async function loadItems(feedID = '') {
    if (!activeProfileId.value) return
    const seq = ++loadSeq
    try {
      const loaded = (await Items(activeProfileId.value, feedID)) ?? []
      if (seq !== loadSeq) return
      loadError.value = null
      items.value = loaded
      // Keep the selection when the reloaded list still has it: a background
      // refresh must not yank the item the user is reading.
      if (selectedId.value && loaded.some((item) => item.id === selectedId.value)) return
      const first = (unreadOnly.value ? items.value.find((item) => item.unread) : items.value[0]) ?? null
      selectedId.value = first?.id ?? null
      await loadActions(first)
    } catch (error) {
      if (seq !== loadSeq) return
      console.warn('Unable to load feed items', error)
      loadError.value = loadErrorMessage(error)
      items.value = []
      selectedId.value = null
      actions.value = []
    }
  }

  async function loadProfiles() {
    try {
      profiles.value = (await Profiles()) ?? []
      profilesLoaded.value = true
      profilesError.value = null
      const active = profiles.value.find((profile) => profile.id === activeProfileId.value) ?? profiles.value[0]
      if (active) {
        await selectProfile(active.id)
      } else {
        activeProfileId.value = ''
        items.value = []
        selectedId.value = null
        actions.value = []
      }
    } catch (error) {
      console.warn('Unable to load feed profiles', error)
      // profilesLoaded stays false here: claiming "loaded, empty" would route
      // an existing user into first-run onboarding instead of an error state.
      profilesError.value = 'Could not load your workspaces.'
    }
  }

  async function createProfile(name: string) {
    if (creatingProfile.value) return // re-entry guard: Enter-key can double-submit
    creatingProfile.value = true
    createProfileError.value = null
    try {
      const created = await CreateProfile(name)
      profiles.value = [...profiles.value, created]
      await selectProfile(created.id)
    } catch (error) {
      console.warn('Unable to create profile', error)
      createProfileError.value = error instanceof Error && error.message ? error.message : 'Could not create the workspace.'
    } finally {
      creatingProfile.value = false
    }
  }

  // Deletes the active profile (the sidebar only exposes this for the
  // active one). On success the modal always closes (see App.vue); errors
  // surface as a toast rather than blocking the confirm flow.
  async function deleteProfile(profileID: string): Promise<boolean> {
    if (deletingProfile.value) return false
    deletingProfile.value = true
    try {
      await DeleteProfile(profileID)
    } catch (error) {
      console.warn('Unable to delete profile', error)
      showToast(error instanceof Error && error.message ? error.message : 'Could not delete the profile.', { severity: 'error' })
      return false
    } finally {
      deletingProfile.value = false
    }
    showToast('Profile deleted', { severity: 'success' })
    await reloadProfilesQuietly()
    if (activeProfileId.value === profileID) {
      const next = profiles.value[0]
      if (next) {
        await selectProfile(next.id)
      } else {
        // No profiles left: same shape as a fresh install — profilesLoaded
        // stays true, so App's needsWorkspace computed routes to the
        // existing "create your first workspace" onboarding step.
        activeProfileId.value = ''
        items.value = []
        selectedId.value = null
        actions.value = []
      }
    }
    return true
  }

  async function loadSources() {
    try {
      sources.value = (await Sources()) ?? []
    } catch (error) {
      // Keep whatever list we had; the editor's ≥1-source rule still guards
      // saves, and a retry happens on the next open.
      console.warn('Unable to load sources', error)
    }
  }

  async function createSource(def: SourceDef): Promise<SourceDef | null> {
    if (creatingSource.value) return null
    creatingSource.value = true
    createSourceError.value = null
    try {
      const created = await CreateSource(def)
      sources.value = [...sources.value, created]
      return created
    } catch (error) {
      console.warn('Unable to create source', error)
      createSourceError.value = error instanceof Error && error.message ? error.message : 'Could not create the source.'
      return null
    } finally {
      creatingSource.value = false
    }
  }

  async function loadFeedDef(profileID: string, feedID: string): Promise<FeedDef | null> {
    try {
      return await FeedDefFor(profileID, feedID)
    } catch (error) {
      console.warn('Unable to load feed definition', error)
      saveFeedError.value = error instanceof Error && error.message ? error.message : 'Could not load the feed.'
      return null
    }
  }

  async function createFeed(profileID: string, def: FeedDef): Promise<boolean> {
    return saveFeed(async () => { await CreateFeed(profileID, def) }, 'Feed created')
  }

  async function updateFeed(profileID: string, feedID: string, def: FeedDef): Promise<boolean> {
    return saveFeed(async () => { await UpdateFeed(profileID, feedID, def) }, 'Feed updated')
  }

  // The config:updated event that follows a successful write also refreshes
  // state, but it rides fsnotify; reload optimistically so the sidebar shows
  // the new feed the moment the sheet closes.
  async function saveFeed(write: () => Promise<void>, toastMessage: string): Promise<boolean> {
    if (savingFeed.value) return false // re-entry guard: Enter-key can double-submit
    savingFeed.value = true
    saveFeedError.value = null
    try {
      await write()
    } catch (error) {
      console.warn('Unable to save feed', error)
      saveFeedError.value = error instanceof Error && error.message ? error.message : 'Could not save the feed.'
      return false
    } finally {
      savingFeed.value = false
    }
    showToast(toastMessage, { severity: 'success' })
    await reloadProfilesQuietly()
    return true
  }

  // The sheet stays open on failure (error surfaces as a toast, not the
  // inline saveFeedError callout — delete has no form to point the error
  // at); the caller closes it on success.
  async function deleteFeed(profileID: string, feedID: string): Promise<boolean> {
    if (deletingFeed) return false
    deletingFeed = true
    try {
      await DeleteFeed(profileID, feedID)
    } catch (error) {
      console.warn('Unable to delete feed', error)
      showToast(error instanceof Error && error.message ? error.message : 'Could not delete the feed.', { severity: 'error' })
      return false
    } finally {
      deletingFeed = false
    }
    showToast('Feed deleted', { severity: 'success' })
    // The deleted feed can no longer anchor the sidebar selection.
    if (selection.value.type === 'feed' && selection.value.feedId === feedID) {
      await selectSidebar({ type: 'all' })
    }
    await reloadProfilesQuietly()
    return true
  }

  async function selectProfile(profileID: string) {
    activeProfileId.value = profileID
    unreadOnly.value = false
    await selectSidebar({ type: 'all' })
  }

  async function selectSidebar(nextSelection: SidebarSelection) {
    // Explicit sidebar navigation always leaves the Unread view; the header
    // chip (toggleUnread) is the way to filter within a scope.
    unreadOnly.value = false
    await applySelection(nextSelection)
  }

  // The sidebar "Unread" view: all-scope with the unread filter on.
  async function selectUnreadView() {
    unreadOnly.value = true
    await applySelection({ type: 'all' })
    await reanchorToUnread()
  }

  // With the unread filter on, a read (filtered-out) selection re-anchors to
  // the first unread item so the detail pane matches the visible list.
  async function reanchorToUnread() {
    if (selectedItem.value && !selectedItem.value.unread) {
      const firstUnread = items.value.find((item) => item.unread) ?? null
      selectedId.value = firstUnread?.id ?? null
      await loadActions(firstUnread)
    }
  }

  async function applySelection(nextSelection: SidebarSelection) {
    selection.value = nextSelection
    await loadItems(nextSelection.type === 'feed' ? nextSelection.feedId : '')
  }

  function currentFeedId(): string {
    return selection.value.type === 'feed' ? selection.value.feedId : ''
  }

  async function reloadProfilesQuietly() {
    try {
      profiles.value = (await Profiles()) ?? []
    } catch (error) {
      console.warn('Unable to refresh profiles', error)
    }
  }

  async function selectItem(id: string) {
    selectedId.value = id
    const item = selectedItem.value
    await loadActions(item)
    if (item) await markItemRead(item)
  }

  // Selecting an item is the read action: app-local only, GitHub untouched.
  async function markItemRead(item: FeedItem) {
    if (!item.unread || !activeProfileId.value) return
    try {
      await MarkRead(activeProfileId.value, item.id)
    } catch (error) {
      console.warn('Unable to mark item read', error)
      return
    }
    item.unread = false
    await reloadProfilesQuietly()
  }

  async function toggleUnread() {
    unreadOnly.value = !unreadOnly.value
    if (unreadOnly.value) await reanchorToUnread()
  }

  async function refresh() {
    if (activeProfileId.value) {
      try {
        await Refresh(activeProfileId.value)
      } catch (error) {
        // Refresh fails only when every feed fails; show the failure state
        // rather than silently re-serving the cache.
        console.warn('Unable to refresh from GitHub', error)
        loadError.value = loadErrorMessage(error)
        return
      }
      await reloadProfilesQuietly()
    }
    await loadItems(currentFeedId())
  }

  function notWired() {
    showToast('Not wired up yet')
  }

  async function loadConfig() {
    try {
      const info = await Config()
      // A fresh invalid state (first sight of this error, or a valid→invalid
      // transition) always re-opens the overlay even if an earlier, now-
      // resolved error was dismissed; an unchanged still-broken error
      // respects an existing dismissal so re-opening the sheet doesn't
      // re-interrupt the user.
      if (!info.valid && (!config.value || config.value.valid || config.value.error !== info.error)) {
        configErrorDismissed.value = false
      }
      config.value = info
    } catch (error) {
      console.warn('Unable to load feeds config', error)
      config.value = null
    }
  }

  // The feeds-as-code handoff: put a schema-complete prompt on the
  // clipboard for the user to paste into a coding agent, which edits the
  // YAML the app then hot-reloads.
  async function copyConfigPrompt() {
    try {
      const prompt = await ConfigPrompt()
      await navigator.clipboard.writeText(prompt)
      showToast('Prompt copied — paste it into a coding agent', { severity: 'success' })
    } catch (error) {
      console.warn('Unable to copy config prompt', error)
      showToast('Could not copy the prompt', { severity: 'error' })
    }
  }

  async function copyConfigPath() {
    const path = config.value?.path
    if (!path) return
    try {
      await navigator.clipboard.writeText(path)
      showToast('Path copied', { severity: 'success' })
    } catch (error) {
      console.warn('Unable to copy config path', error)
      showToast('Could not copy the path', { severity: 'error' })
    }
  }

  // Copies the raw validation error text shown in the config-error overlay —
  // the "Copy" affordance next to its VALIDATION DETAIL list.
  async function copyConfigErrors() {
    const text = config.value?.error
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      showToast('Errors copied', { severity: 'success' })
    } catch (error) {
      console.warn('Unable to copy config errors', error)
      showToast('Could not copy the errors', { severity: 'error' })
    }
  }

  // A config hot-reload can rename or remove the active profile or the
  // selected feed; re-anchor rather than showing items of a definition that
  // no longer exists. Errors keep the last-good data on the backend; the
  // config-error overlay (driven by config.valid, refreshed above) is the
  // signal now, not a toast — it needs the user's attention, not a 4s blip.
  async function onConfigUpdated(status: string) {
    await loadConfig()
    if (status !== 'ok') return
    showToast('Feeds config reloaded', { severity: 'success' })
    await reloadProfilesQuietly()
    profilesLoaded.value = true
    const active = profiles.value.find((profile) => profile.id === activeProfileId.value) ?? profiles.value[0]
    if (!active) {
      activeProfileId.value = ''
      items.value = []
      selectedId.value = null
      actions.value = []
      return
    }
    if (active.id !== activeProfileId.value) {
      await selectProfile(active.id)
      return
    }
    const currentSelection = selection.value
    if (currentSelection.type === 'feed' && !active.feeds?.some((feed) => feed.id === currentSelection.feedId)) {
      await selectSidebar({ type: 'all' })
      return
    }
    await loadItems(currentFeedId())
  }

  async function hideWindow() {
    try {
      if (typeof Window !== 'undefined' && typeof Window.Hide === 'function') await Window.Hide()
    } catch (error) {
      console.debug('Window hide is unavailable outside Wails', error)
    }
  }

  // A push refresh must not disturb what the user is looking at: it swaps
  // the profiles list in place (counts) and re-fetches items only for the
  // active profile; the staleness token handles any race with a manual
  // navigation happening at the same moment. The poller already refetched
  // into the backend cache before emitting this event, so this must not call
  // refresh() — that would trigger a second bypass fetch.
  async function onFeedUpdated(profileID: string) {
    await reloadProfilesQuietly()
    if (profileID === activeProfileId.value) await loadItems(currentFeedId())
  }

  let unsubscribeFeed: (() => void) | undefined
  let unsubscribeConfig: (() => void) | undefined

  onMounted(() => {
    unsubscribeFeed = Events.On('feed:updated', (event) => {
      const profileID = Array.isArray(event.data) ? event.data[0] : event.data
      void onFeedUpdated(typeof profileID === 'string' ? profileID : '')
    })
    unsubscribeConfig = Events.On('config:updated', (event) => {
      const status = Array.isArray(event.data) ? event.data[0] : event.data
      void onConfigUpdated(typeof status === 'string' ? status : 'ok')
    })
    void loadProfiles()
    // Loaded eagerly (not just when the config sheet/feed editor opens) so a
    // broken config surfaces the blocking overlay right at startup.
    void loadConfig()
  })

  onUnmounted(() => {
    unsubscribeFeed?.()
    unsubscribeConfig?.()
    clearToasts()
  })

  return {
    profiles,
    profilesLoaded,
    profilesError,
    activeProfile,
    activeProfileId,
    selection,
    items,
    loadError,
    selectedId,
    selectedItem,
    actions,
    unreadOnly,
    title,
    countLabel,
    toasts,
    showToast,
    dismissToast,
    clearToasts,
    creatingProfile,
    createProfileError,
    deletingProfile,
    sources,
    creatingSource,
    createSourceError,
    savingFeed,
    saveFeedError,
    loadSources,
    createSource,
    loadFeedDef,
    createFeed,
    updateFeed,
    deleteFeed,
    config,
    configErrorOverlayOpen,
    dismissConfigError,
    loadConfig,
    copyConfigPrompt,
    copyConfigPath,
    copyConfigErrors,
    loadProfiles,
    createProfile,
    deleteProfile,
    selectProfile,
    selectSidebar,
    selectUnreadView,
    selectItem,
    toggleUnread,
    refresh,
    notWired,
    hideWindow,
  }
}

// loadErrorMessage maps backend error text onto the designed failure states.
function loadErrorMessage(error: unknown): string {
  const raw = error instanceof Error ? error.message : String(error)
  if (raw.includes('rate limited')) return 'GitHub rate limit hit. Waiting it out.'
  if (raw.includes('not authenticated') || raw.includes('unauthorized')) return 'GitHub session expired. Reconnect from settings.'
  return "Can't reach GitHub right now."
}
