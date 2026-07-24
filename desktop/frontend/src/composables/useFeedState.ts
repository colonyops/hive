import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useStorage } from '@vueuse/core'
import { Browser, Window } from '@wailsio/runtime'
import { CreateFlow, DeleteFlow, GetFlow, GetSidebar, ListFlows, RenameFlow, SaveSidebar, SetFlowEnabled } from '../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { ActionRun, ActionViews, FeedCounts, InboxItemEvents, InvokeAction, ListArchivedInboxItemsByFeed, ListInboxItemsByFeed, ListInboxItemsTrash, MarkInboxItemUnread, SessionLaunchOptions, ToggleInboxItemArchived, ToggleInboxItemIgnored } from '../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import type { ActionRunView, SessionLaunchOptions as SessionLaunchOptionsView } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'
import { bodySnippet, feedSource, githubPayload, typeLabel } from '../lib/feedPresentation'
import { useActivity } from './useActivity'
import { useWailsEvent } from './useWailsEvent'
import { buildFeedTree, treeToLayout } from '../lib/feedTree'
import type { ActionView } from '../types/action'
import type { FeedInboxCount, FeedSort, InboxEvent, InboxItem, FeedSummary, FeedTree, Profile, SidebarSelection } from '../types/feed'
import type { ToastInstance, ToastOptions } from '../types/toast'

// A profile IS a flow: the profiles list comes from FlowsService.ListFlows, a
// profile's sidebar feeds are flow feed nodes, while inbox items are shared
// source identities whose feed membership is represented by claims. The old
// profiles.yaml + feed/source CRUD are gone — editing a source or feed is
// editing its node in the flow canvas.
//
const feedSortStorageKey = 'hive.feed.sort'

// Inbox rows and their membership claims are written by the flow graph runtime
// manager — an app-level instance owned by pipeline/composables/useFlowsSession.ts,
// not this module. It runs every enabled flow independently; refresh() here
// re-reads the selected profile after App.vue's "log:appended" handler has
// pumped all committed work.
export function useFeedState() {
  const profiles = ref<Profile[]>([])
  const profilesLoaded = ref(false)
  const profilesError = ref<string | null>(null)
  const activeProfileId = ref('')
  // Feeds are the only primary destinations. Trash is the fallback selection
  // only when a profile has no feeds at all.
  const selection = ref<SidebarSelection>({ type: 'trash' })

  const unreadOnly = ref(false)
  const feedSort = useStorage<FeedSort>(feedSortStorageKey, 'newest')

  // Last-selected sidebar destination per profile: reopening a workspace
  // returns to where the user left off, else the first feed. Plain
  // localStorage (not useStorage) — it is read once per profile switch and
  // needs no reactivity or cross-instance cache.
  const lastSelectionStorageKey = 'hive.sidebar.last-selection'
  function readLastSelections(): Record<string, SidebarSelection> {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(lastSelectionStorageKey) ?? '{}')
      if (!stored || Array.isArray(stored) || typeof stored !== 'object') return {}
      return stored as Record<string, SidebarSelection>
    } catch {
      return {}
    }
  }
  function writeLastSelection(profileID: string, sel: SidebarSelection): void {
    try { localStorage.setItem(lastSelectionStorageKey, JSON.stringify({ ...readLastSelections(), [profileID]: sel })) }
    catch (error) { console.warn('Unable to persist sidebar selection', error) }
  }
  // Search is a pure view filter over the loaded list (like unreadOnly). It
  // lives here — not in FeedList — so keyboard navigation moves over exactly
  // the rows the user sees. Cleared on feed/profile switch (see selectSidebar).
  const search = ref('')
  const items = ref<InboxItem[]>([])
  // The selected feed's archived section: collapsed by default, lazy-loaded
  // when expanded. Never populated for trash.
  const archivedItems = ref<InboxItem[]>([])
  const archivedExpanded = ref(false)
  const loadError = ref<string | null>(null)
  const selectedId = ref<number | null>(null)
  const actions = ref<ActionView[]>([])
  const pendingActionKeys = ref<Record<string, boolean>>({})
  const actionError = ref<string | null>(null)
  const actionRunsByItem = ref<Record<number, Record<string, ActionRunView>>>({})
  const actionRunGenerations = new Map<string, number>()
  const actionRunIDs = loadActionRunIDs()
  const pendingAction = computed(() => selectedId.value ? Object.keys(pendingActionKeys.value).find((key) => key.startsWith(`${selectedId.value}\u0000`))?.split('\u0000')[1] ?? null : null)
  const actionRuns = computed(() => selectedId.value ? actionRunsByItem.value[selectedId.value] ?? {} : {})
  const sessionLaunchAction = ref<ActionView | null>(null)
  const sessionLaunchItem = ref<InboxItem | null>(null)
  const sessionLaunchOptions = ref<SessionLaunchOptionsView | null>(null)
  const sessionLaunchBusy = ref(false)
  const sessionLaunchError = ref<string | null>(null)
  const toasts = ref<ToastInstance[]>([])
  const creatingProfile = ref(false)
  const createProfileError = ref<string | null>(null)
  const renamingProfile = ref(false)
  const renameProfileError = ref<string | null>(null)
  const togglingProfileId = ref<string | null>(null)
  const toggleProfileError = ref<string | null>(null)
  const deletingProfile = ref(false)
  let nextToastId = 1
  const toastTimers = new Map<number, ReturnType<typeof setTimeout>>()
  const defaultToastDuration = 4000
  // Monotonic token: out-of-order loadItems responses must not clobber newer.
  let loadSeq = 0
  let feedsSeq = 0
  let profilesSeq = 0
  let actionLoadSeq = 0

  function actionKey(itemID: number, actionID: string): string { return `${itemID}\u0000${actionID}` }
  function loadActionRunIDs(): Record<string, Record<string, number>> {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem('hive.action-run-ids') ?? '{}')
      if (!stored || Array.isArray(stored) || typeof stored !== 'object') return {}
      const result: Record<string, Record<string, number>> = {}
      for (const [itemID, itemRuns] of Object.entries(stored)) {
        if (!itemID || !itemRuns || Array.isArray(itemRuns) || typeof itemRuns !== 'object') continue
        const validRuns: Record<string, number> = {}
        for (const [actionID, commandID] of Object.entries(itemRuns)) {
          if (!actionID || typeof commandID !== 'number' || !Number.isSafeInteger(commandID) || commandID <= 0) continue
          validRuns[actionID] = commandID
        }
        if (Object.keys(validRuns).length) result[itemID] = validRuns
      }
      return result
    } catch (error) {
      console.warn('Unable to restore action run IDs', error)
      return {}
    }
  }
  function persistActionRunIDs(): void {
    try { localStorage.setItem('hive.action-run-ids', JSON.stringify(actionRunIDs)) }
    catch (error) { console.warn('Unable to persist action run IDs', error) }
  }
  function nextActionRunGeneration(itemID: number, actionID: string): void {
    const key = actionKey(itemID, actionID)
    actionRunGenerations.set(key, (actionRunGenerations.get(key) ?? 0) + 1)
  }
  function isCurrentActionRun(itemID: number, actionID: string, commandID: number, generation: number): boolean {
    return actionRunIDs[itemID]?.[actionID] === commandID && (actionRunGenerations.get(actionKey(itemID, actionID)) ?? 0) === generation
  }
  function setActionRun(itemID: number, actionID: string, run: ActionRunView): void {
    actionRunsByItem.value = { ...actionRunsByItem.value, [itemID]: { ...(actionRunsByItem.value[itemID] ?? {}), [actionID]: run } }
    actionRunIDs[itemID] = { ...(actionRunIDs[itemID] ?? {}), [actionID]: run.commandId }
    nextActionRunGeneration(itemID, actionID)
    persistActionRunIDs()
  }
  function removeActionRunID(itemID: number, actionID: string): void {
    if (!actionRunIDs[itemID]?.[actionID]) return
    const itemRuns = { ...actionRunIDs[itemID] }; delete itemRuns[actionID]
    if (Object.keys(itemRuns).length) actionRunIDs[itemID] = itemRuns
    else delete actionRunIDs[itemID]
    nextActionRunGeneration(itemID, actionID)
    persistActionRunIDs()
  }

  const activeProfile = computed(() => profiles.value.find((p) => p.id === activeProfileId.value) ?? null)
  const selectedItem = computed(() =>
    items.value.find((item) => item.id === selectedId.value)
      ?? archivedItems.value.find((item) => item.id === selectedId.value)
      ?? null,
  )
  const title = computed(() => {
    const sel = selection.value
    if (sel.type === 'feed') return activeProfile.value?.feeds.find((f) => f.id === sel.feedId)?.name ?? 'Feed'
    return 'Trash'
  })
  const selectedFeed = computed(() => {
    const sel = selection.value
    if (sel.type !== 'feed') return null
    return activeProfile.value?.feeds.find((f) => f.id === sel.feedId) ?? null
  })
  // The archived divider count comes from feed counts so it is correct even
  // while the archived section is collapsed and unloaded.
  const archivedCount = computed(() => selectedFeed.value?.archivedCount ?? 0)
  // Trash filter: 'ignored' narrows to user-ignored items (to un-ignore);
  // 'all' also includes unrouted observations.
  const trashFilter = ref<'all' | 'ignored'>('all')

  // The Unread badge counts the whole loaded list, independent of search.
  const unreadCount = computed(() => items.value.filter((item) => item.unread).length)

  function matchesSearch(item: InboxItem): boolean {
    const query = search.value.trim().toLowerCase()
    if (!query) return true
    const github = githubPayload(item)
    const haystack = [item.title, github.repo, github.author, typeLabel(github.kind), feedSource(item).label, bodySnippet(github.body)]
      .join(' ')
      .toLowerCase()
    return haystack.includes(query)
  }

  function compareItems(a: InboxItem, b: InboxItem): number {
    if (feedSort.value === 'unread' && a.unread !== b.unread) return a.unread ? -1 : 1
    const recency = b.lastEventAt - a.lastEventAt
    return feedSort.value === 'oldest' ? -recency : recency
  }

  const visibleItems = computed(() =>
    [...items.value]
      .sort(compareItems)
      .filter((item) => (!unreadOnly.value || item.unread) && matchesSearch(item) && matchesTrashFilter(item)),
  )

  const visibleArchivedItems = computed(() =>
    [...archivedItems.value].filter((item) => matchesSearch(item)),
  )

  function matchesTrashFilter(item: InboxItem): boolean {
    if (selection.value.type !== 'trash' || trashFilter.value === 'all') return true
    return item.ignoredAt != null
  }

  function setTrashFilter(value: 'all' | 'ignored'): void {
    trashFilter.value = value
  }

  function setFeedSort(value: FeedSort): void {
    feedSort.value = value
  }

  // ── Toasts ──────────────────────────────────────────────────────────────────

  // Some UI-origin outcomes are worth keeping in the durable Activity log, not
  // just the transient toast — these are events only the frontend knows about,
  // demonstrating that the UI records activity the same way the backend does.
  const { record: recordActivity } = useActivity()

  function showToast(message: string, options: ToastOptions = {}): number {
    const severity = options.severity ?? 'info'
    const id = nextToastId++
    const duration = severity === 'error' ? null : options.duration ?? defaultToastDuration
    toasts.value = [...toasts.value, { id, message, body: options.body, severity, actions: options.actions ?? [], duration }]
    if (duration !== null) toastTimers.set(id, setTimeout(() => dismissToast(id), duration))
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

  // ── Profiles (flows) ──────────────────────────────────────────────────────────

  function letter(name: string): string {
    const match = name.match(/[a-z0-9]/i)
    return (match?.[0] ?? '?').toUpperCase()
  }

  // A flow summary maps to a profile; feeds are filled in by loadFeeds once
  // the profile is selected (the rail only needs the letter/name).
  function toProfileStub(flow: { id: string; name: string; enabled: boolean }): Profile {
    const name = flow.name || flow.id
    return { id: flow.id, letter: letter(name), name, enabled: flow.enabled, sourceSummary: '', totalCount: 0, unreadCount: 0, feeds: [] }
  }

  async function loadProfiles() {
    const seq = ++profilesSeq
    try {
      const flows = (await ListFlows()) ?? []
      if (seq !== profilesSeq) return
      profiles.value = flows.map(toProfileStub)
      profilesError.value = null
      const active = profiles.value.find((p) => p.id === activeProfileId.value) ?? profiles.value[0]
      if (active) await selectProfile(active.id)
      else clearActive()
      // Route synchronization starts only after this default profile load is
      // complete; otherwise an initial deep link can race its inbox default.
      profilesLoaded.value = true
    } catch (error) {
      if (seq !== profilesSeq) return
      console.warn('Unable to load flows', error)
      profilesError.value = 'Could not load your workspaces.'
    }
  }

  function clearActive() {
    activeProfileId.value = ''
    items.value = []
    selectedId.value = null
    actions.value = []
  }

  async function reloadProfilesQuietly() {
    const seq = ++profilesSeq
    try {
      const flows = (await ListFlows()) ?? []
      if (seq !== profilesSeq) return
      profiles.value = flows.map(toProfileStub)
      if (activeProfileId.value) await loadFeeds(activeProfileId.value)
    } catch (error) {
      console.warn('Unable to refresh flows', error)
    }
  }

  // loadFeeds populates the active profile's sidebar feeds from its flow's
  // feed nodes, with counts derived from inbox membership claims. A deploy (rename, add or
  // remove a feed node) fires flows:updated, which can start this reload while
  // an earlier one is still in flight; the reads below resolve out of order, so
  // a stale reload must not overwrite a fresher one. Guard with a sequence, the
  // same way loadItems does — otherwise the later-resolving read wins even when
  // it read the pre-deploy flow, leaving the sidebar on the old feed label.
  // Returns the feeds it read so callers that need an immediate answer (e.g.
  // selectProfile's default-feed decision) do not depend on profile state that
  // a concurrent superseded reload may not have written.
  async function loadFeeds(flowId: string): Promise<FeedSummary[] | null> {
    const seq = ++feedsSeq
    try {
      const [flow, counts, sidebar] = await Promise.all([GetFlow(flowId), FeedCounts(flowId), GetSidebar(flowId)])
      const countByFeed = new Map((counts ?? []).map((c) => [c.feedId, c]))
      // GetFlow returns the flattened wire shape (see pipeline/lib/wireFlow):
      // a feed node's config fields (icon/description) sit at the top level of
      // the node object alongside id/type/name, not under a `config` key.
      if (seq !== feedsSeq) return null
      const nodes = (flow.nodes ?? []) as Array<{ id: string; type: string; name?: string; icon?: string; description?: string }>
      const feeds: FeedSummary[] = nodes
        .filter((n) => n.type === 'feed')
        .map((n) => {
          const feedId = `${flowId}/${n.id}`
          const c = countByFeed.get(feedId)
          return { id: feedId, name: n.name || n.id, count: c?.total ?? 0, newCount: c?.unread ?? 0, archivedCount: c?.archived ?? 0, icon: n.icon, description: n.description }
        })
      const sourceCount = nodes.filter((n) => n.type === 'github-source').length
      const profile = profiles.value.find((p) => p.id === flowId)
      if (profile) {
        profile.feeds = feeds
        // Rebuild the sidebar tree from current feeds + saved layout on every
        // load so counts stay fresh and added/removed feeds reconcile in.
        profile.tree = buildFeedTree(feeds, sidebar, flowId)
        profile.sourceSummary = `GitHub · ${sourceCount} source${sourceCount === 1 ? '' : 's'}`
        // Workspace rollups derive from feed counts: without an aggregate
        // inbox there is no workspace-wide query to consult.
        profile.totalCount = feeds.reduce((sum, f) => sum + f.count, 0)
        profile.unreadCount = feeds.reduce((sum, f) => sum + f.newCount, 0)
      }
      return feeds
    } catch (error) {
      console.warn('Unable to load flow feeds', error)
      return null
    }
  }

  // reorderFeeds persists a new sidebar grouping/order for a profile: it
  // updates the in-memory tree optimistically, then writes the layout. The
  // saved <id>.sidebar.yaml is keyed by feed node id (see lib/feedTree).
  async function reorderFeeds(flowId: string, tree: FeedTree) {
    const profile = profiles.value.find((p) => p.id === flowId)
    if (profile) profile.tree = tree
    try {
      await SaveSidebar(flowId, treeToLayout(tree, flowId))
    } catch (error) {
      console.warn('Unable to save sidebar layout', error)
      showToast('Could not save the sidebar layout', { severity: 'error' })
      void recordActivity({
        title: 'Sidebar layout save failed',
        body: profile?.name ? `profile ${profile.name}` : '',
        severity: 'error',
      })
    }
  }

  async function createProfile(name: string) {
    if (creatingProfile.value) return
    creatingProfile.value = true
    createProfileError.value = null
    try {
      const created = await CreateFlow(name)
      profiles.value = [...profiles.value, toProfileStub(created)]
      await selectProfile(created.id)
    } catch (error) {
      console.warn('Unable to create flow', error)
      createProfileError.value = error instanceof Error && error.message ? error.message : 'Could not create the workspace.'
    } finally {
      creatingProfile.value = false
    }
  }

  async function renameProfile(profileID: string, name: string): Promise<boolean> {
    if (renamingProfile.value) return false
    const trimmed = name.trim()
    if (!trimmed) {
      renameProfileError.value = 'Profile name cannot be empty.'
      return false
    }

    const current = profiles.value.find((profile) => profile.id === profileID)
    if (current?.name === trimmed) return true

    renamingProfile.value = true
    renameProfileError.value = null
    try {
      const renamed = await RenameFlow(profileID, trimmed)
      const profile = profiles.value.find((candidate) => candidate.id === profileID)
      if (profile) {
        profile.name = renamed.name
        profile.letter = letter(renamed.name)
      }
      showToast('Profile renamed', { severity: 'success' })
      return true
    } catch (error) {
      console.warn('Unable to rename flow', error)
      renameProfileError.value = error instanceof Error && error.message ? error.message : 'Could not rename the profile.'
      return false
    } finally {
      renamingProfile.value = false
    }
  }

  async function setProfileEnabled(profileID: string, enabled: boolean): Promise<boolean> {
    if (togglingProfileId.value) return false
    const current = profiles.value.find((profile) => profile.id === profileID)
    if (!current || current.enabled === enabled) return true

    togglingProfileId.value = profileID
    toggleProfileError.value = null
    try {
      const updated = await SetFlowEnabled(profileID, enabled)
      const profile = profiles.value.find((candidate) => candidate.id === profileID)
      if (profile) profile.enabled = updated.enabled
      showToast(enabled ? 'Profile enabled' : 'Profile disabled', { severity: 'success' })
      return true
    } catch (error) {
      console.warn('Unable to update flow enablement', error)
      toggleProfileError.value = error instanceof Error && error.message ? error.message : 'Could not update the profile.'
      return false
    } finally {
      togglingProfileId.value = null
    }
  }

  async function deleteProfile(profileID: string): Promise<boolean> {
    if (deletingProfile.value) return false
    // Capture the name before the row is gone, for the activity entry.
    const deletedName = profiles.value.find((p) => p.id === profileID)?.name ?? profileID
    deletingProfile.value = true
    try {
      await DeleteFlow(profileID)
    } catch (error) {
      console.warn('Unable to delete flow', error)
      showToast(error instanceof Error && error.message ? error.message : 'Could not delete the profile.', { severity: 'error' })
      void recordActivity({ title: `Couldn't delete profile ${deletedName}`, severity: 'error', category: 'config' })
      return false
    } finally {
      deletingProfile.value = false
    }
    showToast('Profile deleted', { severity: 'success' })
    void recordActivity({ title: `Deleted profile ${deletedName}`, severity: 'success', category: 'config' })
    await reloadProfilesQuietly()
    if (activeProfileId.value === profileID) {
      const next = profiles.value[0]
      if (next) await selectProfile(next.id)
      else clearActive()
    }
    return true
  }

  // ── Items (inbox_item) ───────────────────────────────────────────────────────

  function asInboxItem(view: InboxItem): InboxItem {
    return { ...view, archivedAt: view.archivedAt ?? null, archivedActor: view.archivedActor ?? null, archivedReason: view.archivedReason ?? null, sourceState: view.sourceState ?? null }
  }

  async function loadTrashItems() {
    if (!activeProfileId.value) return
    const seq = ++loadSeq
    try {
      const loaded = (await ListInboxItemsTrash(activeProfileId.value, 500)) ?? []
      if (seq !== loadSeq) return
      loadError.value = null
      items.value = loaded.map(asInboxItem)
      archivedItems.value = []
      const first = items.value[0] ?? null
      if (selectedId.value && items.value.some((item) => item.id === selectedId.value)) return
      selectedId.value = first?.id ?? null
      await loadActions(first)
    } catch (error) { handleLoadError(seq, error) }
  }

  async function loadFeedItems(feedID: string) {
    if (!activeProfileId.value) return
    const seq = ++loadSeq
    try {
      const loaded = (await ListInboxItemsByFeed(activeProfileId.value, feedID, 500)) ?? []
      // The archived section reloads with the active list only while expanded;
      // collapsed sections stay unloaded until the user opens them.
      const archivedLoaded = archivedExpanded.value
        ? (await ListArchivedInboxItemsByFeed(activeProfileId.value, feedID, 500)) ?? []
        : []
      if (seq !== loadSeq) return
      loadError.value = null
      items.value = loaded.map(asInboxItem)
      archivedItems.value = archivedLoaded.map(asInboxItem)
      const first = (unreadOnly.value ? items.value.find((item) => item.unread) : items.value[0]) ?? null
      if (selectedId.value && (items.value.some((item) => item.id === selectedId.value) || archivedItems.value.some((item) => item.id === selectedId.value))) return
      selectedId.value = first?.id ?? null
      await loadActions(first)
    } catch (error) { handleLoadError(seq, error) }
  }

  async function toggleArchivedSection(): Promise<void> {
    if (selection.value.type !== 'feed') return
    archivedExpanded.value = !archivedExpanded.value
    if (!archivedExpanded.value) {
      archivedItems.value = []
      return
    }
    await loadFeedItems(selection.value.feedId)
  }

  function handleLoadError(seq: number, error: unknown) {
    if (seq !== loadSeq) return
    console.warn('Unable to load inbox items', error)
    loadError.value = "Can't load inbox items right now."
    items.value = []; archivedItems.value = []; selectedId.value = null; actions.value = []
  }

  async function loadEvents(itemID: number): Promise<InboxEvent[]> {
    // The storage query returns newest-first for efficient recent-event reads;
    // the observed timeline is deliberately chronological for human reading.
    return ((await InboxItemEvents(itemID, 100) ?? [])
      .map((event) => ({ ...event, summary: event.summary ?? null, detail: event.detail ?? null }))
      .reverse())
  }

  async function loadActions(item: InboxItem | null) {
    const token = ++actionLoadSeq
    if (!item) { actions.value = []; return }
    try {
      const available = (await ActionViews(githubPayload(item).kind)) ?? []
      if (token !== actionLoadSeq || selectedId.value !== item.id) return
      actions.value = available
      await Promise.all(available.map(async (action) => {
        const commandID = actionRunIDs[item.id]?.[action.id]
        if (!commandID) return
        const generation = actionRunGenerations.get(actionKey(item.id, action.id)) ?? 0
        try {
          const run = await ActionRun(commandID)
          if (isCurrentActionRun(item.id, action.id, commandID, generation)) setActionRun(item.id, action.id, run)
        } catch (error) {
          console.warn('Unable to restore action run', error)
          if (/not found|no rows|missing/i.test(error instanceof Error ? error.message : String(error)) && isCurrentActionRun(item.id, action.id, commandID, generation)) removeActionRunID(item.id, action.id)
        }
      }))
    } catch (error) {
      if (token !== actionLoadSeq) return
      console.warn('Unable to load actions', error)
      actions.value = []
    }
  }

  async function selectItem(id: number) {
    selectedId.value = id
    const item = selectedItem.value
    await loadActions(item)
    if (item?.unread) await markItemUnread(item, false)
  }

  // Opens the existing ActionCard run detail for a persisted job. Jobs carry
  // the feed-item key and action id, so no second run-detail UI is needed.
  async function openActionRun(itemID: number, actionID: string, commandID: number): Promise<boolean> {
    const item = items.value.find((candidate) => candidate.id === itemID)
    if (!item) return false
    await selectItem(itemID)
    try {
      const run = await ActionRun(commandID)
      setActionRun(itemID, actionID, run)
      return true
    } catch (error) {
      console.warn('Unable to open action run', error)
      return false
    }
  }

  async function reloadCurrentSelection(): Promise<void> {
    if (selection.value.type === 'feed') await loadFeedItems(selection.value.feedId)
    else await loadTrashItems()
  }

  async function markItemUnread(item: InboxItem, unread: boolean): Promise<void> {
    try {
      const updated = await MarkInboxItemUnread(item.id, item.revision, unread)
      const index = items.value.findIndex((candidate) => candidate.id === item.id)
      if (index >= 0) items.value.splice(index, 1, asInboxItem(updated))
    } catch (error) {
      console.warn('Unable to update inbox item unread state', error)
      await reloadCurrentSelection()
    }
    if (activeProfileId.value) await loadFeeds(activeProfileId.value)
  }

  async function toggleArchive(item: InboxItem): Promise<void> {
    try {
      await ToggleInboxItemArchived(item.id, item.revision)
      // Archiving moves the item between a feed's active list and its
      // archived section, so the current selection always reloads.
      await reloadCurrentSelection()
    } catch (error) {
      console.warn('Unable to toggle inbox item archive state', error)
      await reloadCurrentSelection()
    }
    if (activeProfileId.value) await loadFeeds(activeProfileId.value)
  }

  async function toggleIgnored(item: InboxItem): Promise<void> {
    try {
      await ToggleInboxItemIgnored(item.id, item.revision)
      await reloadCurrentSelection()
    } catch (error) {
      console.warn('Unable to toggle inbox item ignored state', error)
      await reloadCurrentSelection()
    }
    if (activeProfileId.value) await loadFeeds(activeProfileId.value)
  }

  // Move the selection to the next/previous visible item. Navigation walks the
  // full `items` list (whose order and membership are stable) filtered by the
  // same predicate as visibleItems — this keeps the cursor's anchor valid even
  // when selectItem marks the current item read and it drops out of the unread
  // view. Clamps at both ends (no wrap).
  async function moveSelection(delta: 1 | -1) {
    // Keyboard navigation continues into the archived section when expanded.
    const all = archivedExpanded.value ? [...items.value, ...archivedItems.value] : items.value
    const passes = (item: InboxItem) => (!unreadOnly.value || item.unread) && matchesSearch(item) && matchesTrashFilter(item)
    const currentIndex = all.findIndex((item) => item.id === selectedId.value)
    if (currentIndex === -1) {
      const visible = all.filter(passes)
      const target = delta > 0 ? visible[0] : visible[visible.length - 1]
      if (target) await selectItem(target.id)
      return
    }
    for (let i = currentIndex + delta; i >= 0 && i < all.length; i += delta) {
      if (passes(all[i])) {
        await selectItem(all[i].id)
        return
      }
    }
  }

  const selectNext = () => moveSelection(1)
  const selectPrev = () => moveSelection(-1)

  // ── Navigation ──────────────────────────────────────────────────────────────

  async function selectProfile(profileID: string) {
    activeProfileId.value = profileID
    unreadOnly.value = false
    const feeds = await loadFeeds(profileID)
    // A default (fallback) selection is not a user decision — never persist
    // it, or a transient feed-load race would overwrite the remembered one.
    await selectSidebar(defaultSelection(profileID, feeds ?? undefined), { persist: false })
  }

  // Workspace entry point: the previously selected destination if it still
  // exists, else the first feed in sidebar order, else Trash (feedless flow).
  function defaultSelection(profileID: string, feeds?: FeedSummary[]): SidebarSelection {
    const known = feeds ?? profiles.value.find((p) => p.id === profileID)?.feeds ?? []
    const remembered = readLastSelections()[profileID]
    if (remembered?.type === 'trash') return remembered
    if (remembered?.type === 'feed' && known.some((f) => f.id === remembered.feedId)) return remembered
    const firstFeed = known[0]
    return firstFeed ? { type: 'feed', feedId: firstFeed.id } : { type: 'trash' }
  }

  async function selectSidebar(nextSelection: SidebarSelection, options: { persist?: boolean } = {}) {
    unreadOnly.value = false
    search.value = '' // a switched feed starts unfiltered
    trashFilter.value = 'all'
    archivedExpanded.value = false
    archivedItems.value = []
    await applySelection(nextSelection, options)
  }

  async function selectUnreadView() {
    unreadOnly.value = true
    if (selection.value.type !== 'feed') {
      const fallback = defaultSelection(activeProfileId.value)
      if (fallback.type === 'feed') await applySelection(fallback)
    }
    await reanchorToUnread()
  }

  async function reanchorToUnread() {
    if (selectedItem.value && !selectedItem.value.unread) {
      const firstUnread = items.value.find((item) => item.unread) ?? null
      selectedId.value = firstUnread?.id ?? null
      await loadActions(firstUnread)
    }
  }

  async function applySelection(nextSelection: SidebarSelection, options: { persist?: boolean } = {}) {
    selection.value = nextSelection
    if (options.persist !== false && activeProfileId.value) writeLastSelection(activeProfileId.value, nextSelection)
    if (nextSelection.type === 'feed') await loadFeedItems(nextSelection.feedId)
    else await loadTrashItems()
  }

  async function refreshCurrent(): Promise<void> {
    await reloadCurrentSelection()
  }

  async function toggleUnread() {
    unreadOnly.value = !unreadOnly.value
    if (unreadOnly.value) await reanchorToUnread()
  }

  async function refresh() {
    if (activeProfileId.value) await loadFeeds(activeProfileId.value)
    await refreshCurrent()
  }

  async function runAction(actionID: string, input: Record<string, unknown> = {}, item = selectedItem.value) {
    if (!item) return false
    const key = actionKey(item.id, actionID)
    if (pendingActionKeys.value[key]) return false
    pendingActionKeys.value = { ...pendingActionKeys.value, [key]: true }
    actionError.value = null
    try {
      const run = await InvokeAction(actionID, item.id, input)
      setActionRun(item.id, actionID, run)
      if (run.status !== 'done') {
        actionError.value = run.error || 'The action did not complete.'
        showToast(actionError.value, { severity: 'error' })
        return false
      }
      const label = actions.value.find((action) => action.id === actionID)?.label ?? actionID
      if (run.result?.session) showToast(`Created session ${run.result.session.name} (${run.result.session.id})`, { severity: 'success' })
      else if (run.result?.message) showToast(`Published message to ${run.result.message.topic} as ${run.result.message.sender}`, { severity: 'success' })
      else showToast(`${label} completed`, { severity: 'success' })
      return true
    } catch (error) {
      console.warn('Unable to invoke action', error)
      actionError.value = error instanceof Error && error.message ? error.message : 'Could not run the action.'
      showToast(actionError.value, { severity: 'error' })
      return false
    } finally {
      const next = { ...pendingActionKeys.value }; delete next[key]; pendingActionKeys.value = next
    }
  }

  // Interactive launch-session actions never invoke until the user has chosen
  // a repository and valid session name. Configured repo_template actions
  // remain headless and use the same direct execution path as other actions.
  async function invokeAction(actionID: string) {
    const action = actions.value.find((candidate) => candidate.id === actionID)
    if (!action?.requiresSessionInput) {
      await runAction(actionID)
      return
    }
    if (pendingAction.value || sessionLaunchBusy.value) return
    const item = selectedItem.value
    if (!item) return
    const key = actionKey(item.id, actionID)
    sessionLaunchError.value = null
    pendingActionKeys.value = { ...pendingActionKeys.value, [key]: true }
    try {
      sessionLaunchOptions.value = await SessionLaunchOptions()
      sessionLaunchAction.value = action
      sessionLaunchItem.value = item
    } catch (error) {
      const message = error instanceof Error && error.message ? error.message : 'Could not load session options.'
      actionError.value = message
      showToast(message, { severity: 'error' })
    } finally {
      const next = { ...pendingActionKeys.value }; delete next[key]; pendingActionKeys.value = next
    }
  }

  function cancelSessionLaunch() {
    if (sessionLaunchBusy.value) return
    sessionLaunchAction.value = null
    sessionLaunchItem.value = null
    sessionLaunchOptions.value = null
    sessionLaunchError.value = null
  }

  async function submitSessionLaunch(input: { name: string; repository: string; agent?: string }) {
    const action = sessionLaunchAction.value
    const item = sessionLaunchItem.value
    if (!action || !item || sessionLaunchBusy.value) return
    sessionLaunchBusy.value = true
    sessionLaunchError.value = null
    const succeeded = await runAction(action.id, { session: input }, item)
    sessionLaunchBusy.value = false
    if (succeeded) cancelSessionLaunch()
    else sessionLaunchError.value = actionError.value ?? 'Could not create the session.'
  }

  function notWired() {
    showToast('Not wired up yet')
  }

  // Opens a URL in the user's default browser via Wails — used for links
  // rendered inside a feed item's markdown body.
  async function openUrl(url: string) {
    if (!url) return
    try {
      await Browser.OpenURL(url)
    } catch (error) {
      console.warn('Unable to open URL', error)
      showToast('Could not open the link', { severity: 'error' })
    }
  }

  // Opens the selected item's canonical GitHub URL (the detail pane's "open"
  // button). The URL comes from the inbox item; a missing one means the
  // source did not carry it.
  async function openSelectedInBrowser() {
    const url = selectedItem.value?.url
    if (!url) {
      showToast('No link available for this item', { severity: 'error' })
      return
    }
    await openUrl(url)
  }

  async function hideWindow() {
    try {
      if (typeof Window !== 'undefined' && typeof Window.Hide === 'function') await Window.Hide()
    } catch (error) {
      console.debug('Window hide is unavailable outside Wails', error)
    }
  }

  // ── Wails events ──────────────────────────────────────────────────────────────
  // Note: no "log:appended" listener here — App.vue owns that subscription
  // now (see pipeline/composables/useFlowsSession.ts), since the runtime's
  // commit into inbox rows/claims must complete BEFORE this module's refresh() reads
  // it back. Subscribing here too would race that commit and could read
  // stale inbox rows.

  onMounted(() => {
    // A flows/*.yaml change (create/delete/edit) reshapes the profiles list.
    useWailsEvent('flows:updated', () => { void reloadProfilesQuietly() })
    useWailsEvent('actions:updated', () => { void loadActions(selectedItem.value) })
    void loadProfiles()
  })

  onUnmounted(() => {
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
    visibleItems,
    visibleArchivedItems,
    archivedExpanded,
    archivedCount,
    toggleArchivedSection,
    trashFilter,
    setTrashFilter,
    unreadCount,
    search,
    loadError,
    selectedId,
    selectedItem,
    actions,
    pendingAction,
    actionError,
    actionRuns,
    sessionLaunchAction,
    sessionLaunchOptions,
    sessionLaunchBusy,
    sessionLaunchError,
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
    loadProfiles,
    createProfile,
    renameProfile,
    setProfileEnabled,
    deleteProfile,
    reorderFeeds,
    selectProfile,
    defaultSelection,
    selectSidebar,
    selectUnreadView,
    selectItem,
    openActionRun,
    selectNext,
    selectPrev,
    toggleUnread,
    markItemUnread,
    toggleArchive,
    toggleIgnored,
    loadEvents,
    refresh,
    invokeAction,
    cancelSessionLaunch,
    submitSessionLaunch,
    notWired,
    openUrl,
    openSelectedInBrowser,
    hideWindow,
  }
}
