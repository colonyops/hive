import { computed, onMounted, ref } from 'vue'
import { useStorage } from '@vueuse/core'
import { Browser, Window } from '@wailsio/runtime'
import { ClearProfileImage, CreateFlow, DeleteFlow, GetFlow, GetSidebar, ListFlows, MarkImages, RenameFlow, SaveSidebar, SeedStarterFlow, SetFlowEnabled, SetFlowOrder, SetProfileImage } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/flowsservice'
import { ActionRun, ActionViews, Events, FeedCounts, InvokeAction, ListArchivedByFeed, ListByFeed, ListTrash, MarkRead, RenderClipboardAction, SetUnread, ToggleArchived, ToggleIgnored } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import { SessionLaunchOptions } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import { Refresh as RefreshSources } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sourcesservice'
import type { ActionRunView, SessionLaunchOptions as SessionLaunchOptionsView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { appErrorKind, appErrorMessage, errorText } from '../lib/appError'
import { canonicalPayload, clipboardText, searchText, sourceKindForNodeType, sourceSummary } from '../lib/itemPresentation'
import { useClipboard } from './useClipboard'
import { useNotify } from './useNotify'
import { useToasts } from './useToasts'
import { useWailsEvent } from './useWailsEvent'
import { buildFeedTree, treeToLayout } from '../lib/feedTree'
import type { ActionView } from '../types/action'
import type { FeedInboxCount, FeedSort, InboxEvent, InboxItem, FeedSummary, FeedTree, Profile, SidebarSelection } from '../types/feed'

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
  // True while a manual source fetch is in flight, so the refresh control can
  // say so. A poll tick is not covered: it is not something the user started.
  const refreshingSources = ref(false)
  const feedSort = useStorage<FeedSort>(feedSortStorageKey, 'newest')

  // Last-selected sidebar destination per profile: reopening a profile
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
  const authorFilter = ref('')
  const items = ref<InboxItem[]>([])
  // Per-source-node feed icons for the active flow (node id → icon key),
  // read from sources.webhook configs in loadFeeds. Feed rows and the detail
  // pane look up an item's glyph by its sourceScope (the source node id).
  const sourceIcons = ref<Record<string, string>>({})
  // Per-source-node mark images for the active flow (node id → PNG data URL),
  // resolved from sources.webhook `image` hashes in loadFeeds.
  const sourceImages = ref<Record<string, string>>({})
  // The selected feed's archived section: collapsed by default, lazy-loaded
  // when expanded. Never populated for trash.
  const archivedItems = ref<InboxItem[]>([])
  const archivedExpanded = ref(false)
  const loadError = ref<string | null>(null)
  const selectedId = ref<number | null>(null)
  const itemSelectionActive = ref(false)
  const selectedItemIDs = ref<number[]>([])
  const selectionActions = ref<ActionView[]>([])
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
  const actionInputsAction = ref<ActionView | null>(null)
  const actionInputsItems = ref<InboxItem[]>([])
  const actionInputsBusy = ref(false)
  const actionInputsError = ref<string | null>(null)
  const actionRerunConfirmation = ref<{ actionID: string; label: string; item: InboxItem; input: Record<string, unknown> } | null>(null)
  const actionRerunBusy = ref(false)
  const actionRerunError = ref<string | null>(null)
  const { toasts, showToast, dismissToast, clearToasts } = useToasts()
  const { notify } = useNotify()
  const creatingProfile = ref(false)
  const createProfileError = ref<string | null>(null)
  const renamingProfile = ref(false)
  const renameProfileError = ref<string | null>(null)
  const togglingProfileId = ref<string | null>(null)
  const toggleProfileError = ref<string | null>(null)
  const deletingProfile = ref(false)
  const settingProfileImage = ref(false)
  const profileImageError = ref<string | null>(null)
  const markingAllRead = ref(false)
  // Monotonic token: out-of-order loadItems responses must not clobber newer.
  let loadSeq = 0
  let feedsSeq = 0
  let profilesSeq = 0
  let selectionActionsSeq = 0
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

  const authors = computed(() => [...new Set(
    [...items.value, ...archivedItems.value].map((item) => canonicalPayload(item).author.trim()).filter(Boolean),
  )].sort((a, b) => a.localeCompare(b)))

  function matchesAuthor(item: InboxItem): boolean {
    return !authorFilter.value || canonicalPayload(item).author.trim() === authorFilter.value
  }

  function matchesSearch(item: InboxItem): boolean {
    const query = search.value.trim().toLowerCase()
    if (!query) return true
    return searchText(item).toLowerCase().includes(query)
  }

  function compareItems(a: InboxItem, b: InboxItem): number {
    if (feedSort.value === 'unread' && a.unread !== b.unread) return a.unread ? -1 : 1
    const recency = b.lastEventAt - a.lastEventAt
    return feedSort.value === 'oldest' ? -recency : recency
  }

  const visibleItems = computed(() =>
    [...items.value]
      .sort(compareItems)
      .filter((item) => (!unreadOnly.value || item.unread) && matchesSearch(item) && matchesAuthor(item) && matchesTrashFilter(item)),
  )

  const visibleArchivedItems = computed(() =>
    [...archivedItems.value].filter((item) => matchesSearch(item) && matchesAuthor(item)),
  )

  const selectedItems = computed(() => {
    const chosen = new Set(selectedItemIDs.value)
    return [...items.value].sort(compareItems).concat(archivedItems.value).filter((item) => chosen.has(item.id))
  })

  async function loadSelectionActions(): Promise<void> {
    const seq = ++selectionActionsSeq
    const itemIDs = selectedItems.value.map((item) => item.id)
    selectionActions.value = []
    if (!itemSelectionActive.value || itemIDs.length === 0) return
    try {
      const actionsByItem = await Promise.all(itemIDs.map(async (itemID) => (await ActionViews(itemID)) ?? []))
      const [first = [], ...rest] = actionsByItem
      const common = first.filter((action) => action.type === 'clipboard' && rest.every((itemActions) => itemActions.some((candidate) => candidate.id === action.id)))
      if (seq === selectionActionsSeq) selectionActions.value = common
    } catch (error) {
      if (seq !== selectionActionsSeq) return
      console.warn('Unable to load clipboard actions for item selection', error)
      selectionActions.value = []
    }
  }

  function enterItemSelection(): void {
    itemSelectionActive.value = true
  }

  function cancelItemSelection(): void {
    itemSelectionActive.value = false
    selectedItemIDs.value = []
    selectionActions.value = []
    selectionActionsSeq++
  }

  function toggleItemSelection(itemID: number): void {
    if (!itemSelectionActive.value) return
    selectedItemIDs.value = selectedItemIDs.value.includes(itemID)
      ? selectedItemIDs.value.filter((id) => id !== itemID)
      : [...selectedItemIDs.value, itemID]
    void loadSelectionActions()
  }

  function pruneItemSelection(): void {
    const available = new Set([...items.value, ...archivedItems.value].map((item) => item.id))
    selectedItemIDs.value = selectedItemIDs.value.filter((id) => available.has(id))
    void loadSelectionActions()
  }

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

  // ── Profiles (flows) ──────────────────────────────────────────────────────────

  function letter(name: string): string {
    const match = name.match(/[a-z0-9]/i)
    return (match?.[0] ?? '?').toUpperCase()
  }

  // A flow summary maps to a profile; feeds are filled in by loadFeeds once
  // the profile is selected (the rail only needs the letter/name). An
  // undefined tree is therefore "feeds not read yet", which is distinct from
  // a profile whose flow genuinely has no feed nodes.
  function toProfileStub(flow: { id: string; name: string; enabled: boolean; image?: string; nodes?: number }): Profile {
    const name = flow.name || flow.id
    return { id: flow.id, letter: letter(name), image: flow.image || undefined, name, enabled: flow.enabled, nodes: flow.nodes ?? 0, sourceSummary: '', totalCount: 0, unreadCount: 0, feeds: [] }
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
      profilesError.value = 'Could not load your profiles.'
    }
  }

  function clearActive() {
    activeProfileId.value = ''
    items.value = []
    selectedId.value = null
    actions.value = []
  }

  // Before the first load lands, a quiet reload is the first load: it takes
  // over profilesSeq, so the load it supersedes returns without selecting a
  // profile or setting profilesLoaded, and the shell would wait on it forever.
  async function reloadProfilesQuietly() {
    if (!profilesLoaded.value) return await loadProfiles()
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
      const nodes = (flow.nodes ?? []) as Array<{ id: string; type: string; name?: string; icon?: string; image?: string; description?: string }>
      const feeds: FeedSummary[] = nodes
        .filter((n) => n.type === 'feed')
        .map((n) => {
          const feedId = `${flowId}/${n.id}`
          const c = countByFeed.get(feedId)
          return { id: feedId, name: n.name || n.id, count: c?.total ?? 0, newCount: c?.unread ?? 0, archivedCount: c?.archived ?? 0, icon: n.icon, description: n.description }
        })
      const icons: Record<string, string> = {}
      const imageHashByNode: Record<string, string> = {}
      const countByKind = new Map<string, number>()
      for (const n of nodes) {
        const nodeSourceKind = sourceKindForNodeType(n.type)
        if (!nodeSourceKind) continue
        // Keyed by node id because that is the sourceScope every source node
        // carrying its own mark ingests under.
        if (n.icon) icons[n.id] = n.icon
        if (n.image) imageHashByNode[n.id] = n.image
        countByKind.set(nodeSourceKind, (countByKind.get(nodeSourceKind) ?? 0) + 1)
      }
      sourceIcons.value = icons
      // Resolve mark hashes to data URLs; a stale resolve is dropped by feedsSeq.
      const hashes = [...new Set(Object.values(imageHashByNode))]
      if (hashes.length === 0) {
        sourceImages.value = {}
      } else {
        let resolved: Record<string, string | undefined> = {}
        try {
          resolved = (await MarkImages(hashes)) ?? {}
        } catch (error) {
          console.warn('Unable to load source mark images', error)
        }
        if (seq !== feedsSeq) return null
        const images: Record<string, string> = {}
        for (const [nodeId, hash] of Object.entries(imageHashByNode)) {
          const url = resolved[hash]
          if (url) images[nodeId] = url
        }
        sourceImages.value = images
      }
      const profile = profiles.value.find((p) => p.id === flowId)
      if (profile) {
        profile.feeds = feeds
        // Rebuild the sidebar tree from current feeds + saved layout on every
        // load so counts stay fresh and added/removed feeds reconcile in.
        profile.tree = buildFeedTree(feeds, sidebar, flowId)
        profile.sourceSummary = sourceSummary(countByKind)
        // Profile rollups derive from feed counts: without an aggregate inbox
        // there is no profile-wide query to consult.
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
      await notify({
        title: 'Sidebar layout save failed',
        body: profile?.name ? `Could not save the sidebar layout for profile ${profile.name}.` : 'Could not save the sidebar layout.',
        severity: 'error',
        category: 'config',
      })
    }
  }

  // reorderProfiles persists the rail order (settings.yaml's profiles.order),
  // reordering the local list first so the tile stays where it was dropped
  // instead of snapping back until the write answers. The backend's
  // flows:updated confirms it; a failed write puts the old order back, since
  // nothing else will.
  async function reorderProfiles(ids: string[]) {
    const previous = profiles.value
    const byID = new Map(previous.map((p) => [p.id, p]))
    const reordered = ids.map((id) => byID.get(id)).filter((p): p is Profile => !!p)
    if (reordered.length !== previous.length) return
    profiles.value = reordered
    try {
      await SetFlowOrder(ids)
    } catch (error) {
      profiles.value = previous
      console.warn('Unable to save the profile order', error)
      await notify({
        title: 'Profile order save failed',
        body: 'Could not save the order of the profile rail.',
        severity: 'error',
        category: 'config',
      })
    }
  }

  // Returns the new profile's id, or null if it could not be created —
  // onboarding needs it to seed the profile it just made once GitHub is
  // connected, and reading it back off activeProfileId would depend on a
  // selection a concurrent reload could have moved.
  async function createProfile(name: string): Promise<string | null> {
    if (creatingProfile.value) return null
    creatingProfile.value = true
    createProfileError.value = null
    try {
      const created = await CreateFlow(name)
      profiles.value = [...profiles.value, toProfileStub(created)]
      await selectProfile(created.id)
      return created.id
    } catch (error) {
      console.warn('Unable to create flow', error)
      createProfileError.value = error instanceof Error && error.message ? error.message : 'Could not create the profile.'
      return null
    } finally {
      creatingProfile.value = false
    }
  }

  // seedStarterFlow fills an empty profile with the starter graph. The
  // backend also publishes flows:updated, but this reload is what makes the
  // sidebar's feeds readable by the time the caller continues, rather than
  // whenever the event lands.
  async function seedStarterFlow(profileID: string): Promise<void> {
    await SeedStarterFlow(profileID)
    await reloadProfilesQuietly()
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
      await notify({ title: 'Profile renamed', body: renamed.name, severity: 'success', category: 'config' })
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
      await notify({ title: enabled ? 'Profile enabled' : 'Profile disabled', body: updated.name, severity: 'success', category: 'config' })
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
    // Capture the name before the row is gone, for the success notification.
    const deletedName = profiles.value.find((p) => p.id === profileID)?.name ?? profileID
    deletingProfile.value = true
    try {
      await DeleteFlow(profileID)
    } catch (error) {
      console.warn('Unable to delete flow', error)
      await notify({
        title: `Couldn't delete profile ${deletedName}`,
        body: error instanceof Error && error.message ? error.message : 'Could not delete the profile.',
        severity: 'error',
        category: 'config',
      })
      return false
    } finally {
      deletingProfile.value = false
    }
    await notify({ title: 'Profile deleted', body: deletedName, severity: 'success', category: 'config' })
    await reloadProfilesQuietly()
    if (activeProfileId.value === profileID) {
      const next = profiles.value[0]
      if (next) await selectProfile(next.id)
      else clearActive()
    }
    return true
  }

  // setProfileImage ships the picked file (already base64) to the backend,
  // which normalizes and stores it, then reloads so the rail and settings
  // preview pick up the new avatar. reloadProfilesQuietly rebuilds profiles
  // from summaries, whose image field the backend now populates.
  async function setProfileImage(profileID: string, data: string): Promise<boolean> {
    if (settingProfileImage.value) return false
    settingProfileImage.value = true
    profileImageError.value = null
    try {
      await SetProfileImage(profileID, data)
      await reloadProfilesQuietly()
      return true
    } catch (error) {
      console.warn('Unable to set profile image', error)
      profileImageError.value = appErrorMessage(error) || (error instanceof Error ? error.message : '') || 'Could not update the profile image.'
      return false
    } finally {
      settingProfileImage.value = false
    }
  }

  async function clearProfileImage(profileID: string): Promise<boolean> {
    if (settingProfileImage.value) return false
    settingProfileImage.value = true
    profileImageError.value = null
    try {
      await ClearProfileImage(profileID)
      await reloadProfilesQuietly()
      return true
    } catch (error) {
      console.warn('Unable to clear profile image', error)
      profileImageError.value = appErrorMessage(error) || (error instanceof Error ? error.message : '') || 'Could not remove the profile image.'
      return false
    } finally {
      settingProfileImage.value = false
    }
  }

  // ── Items (inbox_item) ───────────────────────────────────────────────────────

  function asInboxItem(view: InboxItem): InboxItem {
    return { ...view, archivedAt: view.archivedAt ?? null, archivedActor: view.archivedActor ?? null, archivedReason: view.archivedReason ?? null, sourceState: view.sourceState ?? null }
  }

  async function loadTrashItems() {
    if (!activeProfileId.value) return
    const seq = ++loadSeq
    try {
      const loaded = (await ListTrash(activeProfileId.value, 500)) ?? []
      if (seq !== loadSeq) return
      loadError.value = null
      items.value = loaded.map(asInboxItem)
      archivedItems.value = []
      pruneItemSelection()
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
      const loaded = (await ListByFeed(activeProfileId.value, feedID, 500)) ?? []
      // Selection can include archived rows after their section is collapsed.
      // Reload them while selecting so pruning distinguishes moved rows from
      // rows that no longer exist.
      const loadArchived = archivedExpanded.value || itemSelectionActive.value
      const archivedLoaded = loadArchived
        ? (await ListArchivedByFeed(activeProfileId.value, feedID, 500)) ?? []
        : []
      if (seq !== loadSeq) return
      loadError.value = null
      items.value = loaded.map(asInboxItem)
      if (loadArchived) archivedItems.value = archivedLoaded.map(asInboxItem)
      pruneItemSelection()
      const first = (unreadOnly.value ? items.value.find((item) => item.unread) : items.value[0]) ?? null
      if (selectedId.value && (items.value.some((item) => item.id === selectedId.value) || archivedItems.value.some((item) => item.id === selectedId.value))) return
      selectedId.value = first?.id ?? null
      await loadActions(first)
    } catch (error) { handleLoadError(seq, error) }
  }

  async function toggleArchivedSection(): Promise<void> {
    if (selection.value.type !== 'feed') return
    archivedExpanded.value = !archivedExpanded.value
    if (!archivedExpanded.value) return
    await loadFeedItems(selection.value.feedId)
  }

  function handleLoadError(seq: number, error: unknown) {
    if (seq !== loadSeq) return
    console.warn('Unable to load inbox items', error)
    loadError.value = "Can't load inbox items right now."
    items.value = []; archivedItems.value = []; selectedId.value = null; actions.value = []
    pruneItemSelection()
  }

  async function loadEvents(itemID: number): Promise<InboxEvent[]> {
    // The storage query returns newest-first for efficient recent-event reads;
    // the observed timeline is deliberately chronological for human reading.
    return ((await Events(itemID, 100) ?? [])
      .map((event) => ({ ...event, summary: event.summary ?? null, detail: event.detail ?? null }))
      .reverse())
  }

  async function loadActions(item: InboxItem | null) {
    const token = ++actionLoadSeq
    if (!item) { actions.value = []; return }
    try {
      const available = (await ActionViews(item.id)) ?? []
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
          // A run row deleted underneath us is not a failure: drop the stale
          // id so the card disappears. Anything else is left alone, because
          // forgetting a run id on a transient fault loses the user's link to
          // work that is still running.
          if (appErrorKind(error) === 'not_found' && isCurrentActionRun(item.id, action.id, commandID, generation)) removeActionRunID(item.id, action.id)
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

  // A write returns the row's new revision; apply it wherever that row is
  // loaded. The archived section holds rows too, and a row selected there is
  // just as writable as an active one — skipping it would leave the stale
  // revision that the next optimistic write (archive toggle) rejects.
  function applyItemUpdate(updated: InboxItem): void {
    const next = asInboxItem(updated)
    for (const list of [items, archivedItems]) {
      const index = list.value.findIndex((candidate) => candidate.id === next.id)
      if (index >= 0) list.value.splice(index, 1, next)
    }
  }

  async function markItemUnread(item: InboxItem, unread: boolean): Promise<void> {
    try {
      applyItemUpdate(await SetUnread(item.id, item.revision, unread))
    } catch (error) {
      console.warn('Unable to update inbox item unread state', error)
      await reloadCurrentSelection()
    }
    if (activeProfileId.value) await loadFeeds(activeProfileId.value)
  }

  // Bulk read-state clear for a whole scope: one feed, or every feed in the
  // profile when feedID is null. One backend write, not N revision-guarded
  // per-item calls — so the loaded rows and the sidebar counts are re-read
  // afterwards rather than patched.
  //
  // It deliberately ignores the search box and the unread filter. Those narrow
  // what is on screen right now; "all" means the scope the user named, and a
  // half-cleared feed whose leftovers depend on a search term nobody will
  // remember typing is the worse surprise.
  async function markAllRead(feedID: string | null = null): Promise<void> {
    if (!activeProfileId.value || markingAllRead.value) return
    markingAllRead.value = true
    try {
      const marked = await MarkRead(activeProfileId.value, feedID ?? '')
      showToast(marked > 0 ? `Marked ${marked} ${marked === 1 ? 'item' : 'items'} as read` : 'Nothing to mark as read', { severity: 'success' })
    } catch (error) {
      console.warn('Unable to mark inbox items as read', error)
      showToast('Could not mark items as read', { severity: 'error' })
    } finally {
      markingAllRead.value = false
    }
    await refresh()
  }

  // The count the confirmation quotes and the sidebar badge shows are the same
  // number: feed counts are the only unread aggregate the app has.
  function unreadInScope(feedID: string | null = null): number {
    if (!feedID) return activeProfile.value?.unreadCount ?? 0
    return activeProfile.value?.feeds.find((feed) => feed.id === feedID)?.newCount ?? 0
  }

  async function toggleArchive(item: InboxItem): Promise<void> {
    try {
      await ToggleArchived(item.id, item.revision)
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
      await ToggleIgnored(item.id, item.revision)
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
    const passes = (item: InboxItem) => (!unreadOnly.value || item.unread) && matchesSearch(item) && matchesAuthor(item) && matchesTrashFilter(item)
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
    if (activeProfileId.value !== profileID) cancelItemSelection()
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
    const destinationChanged = selection.value.type !== nextSelection.type
      || (selection.value.type === 'feed' && nextSelection.type === 'feed' && selection.value.feedId !== nextSelection.feedId)
    if (destinationChanged) cancelItemSelection()
    unreadOnly.value = false
    search.value = '' // a switched feed starts unfiltered
    authorFilter.value = ''
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
    if (selectedItem.value && (!selectedItem.value.unread || !matchesAuthor(selectedItem.value))) {
      const firstUnread = visibleItems.value.find((item) => item.unread) ?? null
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

  // The user-initiated refresh: fetch from every configured source, then
  // re-read. refresh() above is the passive half and stays that way — it is
  // what the "inbox:updated" subscription runs, and fetching from there would
  // make every commit trigger the fetch that produced it.
  //
  // The reload is unconditional. A source that failed still leaves whatever
  // the others appended, and the engine commits after this returns anyway, so
  // the list the user ends up looking at comes from "inbox:updated" either
  // way; this one just guarantees a redraw when nothing new arrived.
  async function refreshSources() {
    if (refreshingSources.value) return
    refreshingSources.value = true
    try {
      await RefreshSources()
    } catch (error) {
      // Mock modes have no producer. There is nothing to fetch and nothing
      // the user can do about it, so fall through to the reload silently.
      if (appErrorKind(error) !== 'unavailable') {
        console.warn('Unable to refresh sources', error)
        await notify({
          title: errorText(error, 'Could not refresh the sources.'),
          severity: 'error',
          category: 'refresh',
        })
      }
    } finally {
      refreshingSources.value = false
    }
    await refresh()
  }

  async function runAction(actionID: string, input: Record<string, unknown> = {}, item = selectedItem.value) {
    if (!item) return false
    const key = actionKey(item.id, actionID)
    if (pendingActionKeys.value[key]) return false
    pendingActionKeys.value = { ...pendingActionKeys.value, [key]: true }
    actionError.value = null
    try {
      const run = await InvokeAction(actionID, item.id, input)
      if (run.confirmationRequired) {
        const label = actions.value.find((action) => action.id === actionID)?.label ?? actionID
        actionRerunConfirmation.value = { actionID, label, item, input }
        actionRerunError.value = null
        return false
      }
      setActionRun(item.id, actionID, run)
      if (run.status !== 'done') {
        actionError.value = run.error || 'The action did not complete.'
        await notify({ title: actionError.value, severity: 'error', category: 'action' })
        return false
      }
      const label = actions.value.find((action) => action.id === actionID)?.label ?? actionID
      if (run.result?.session) await notify({ title: `Created session ${run.result.session.name} (${run.result.session.id})`, severity: 'success', category: 'session' })
      else if (run.result?.message) await notify({ title: `Published message to ${run.result.message.topic} as ${run.result.message.sender}`, severity: 'success', category: 'action' })
      else await notify({ title: `${label} completed`, severity: 'success', category: 'action' })
      return true
    } catch (error) {
      console.warn('Unable to invoke action', error)
      actionError.value = error instanceof Error && error.message ? error.message : 'Could not run the action.'
      await notify({ title: actionError.value, severity: 'error', category: 'action' })
      return false
    } finally {
      const next = { ...pendingActionKeys.value }; delete next[key]; pendingActionKeys.value = next
    }
  }

  function cancelActionRerun() {
    if (actionRerunBusy.value) return
    actionRerunConfirmation.value = null
    actionRerunError.value = null
  }

  async function confirmActionRerun() {
    const pending = actionRerunConfirmation.value
    if (!pending || actionRerunBusy.value) return
    actionRerunBusy.value = true
    actionRerunError.value = null
    const succeeded = await runAction(pending.actionID, { ...pending.input, rerun: true }, pending.item)
    actionRerunBusy.value = false
    if (succeeded) {
      actionRerunConfirmation.value = null
      if (sessionLaunchAction.value) cancelSessionLaunch()
      if (actionInputsAction.value) cancelActionInputs()
    } else {
      actionRerunError.value = actionError.value ?? 'Could not rerun the action.'
    }
  }

  // An action that needs something from the user does not invoke on click: it
  // opens a form first. Declared inputs open the generic one; an interactive
  // launch-session action opens the session dialog, which renders any declared
  // inputs alongside its own fields. Everything else runs directly.
  async function invokeAction(actionID: string) {
    const action = actions.value.find((candidate) => candidate.id === actionID)
    if (action?.inputs?.length && !action.requiresSessionInput) {
      if (pendingAction.value || actionInputsBusy.value) return
      const item = selectedItem.value
      if (!item) return
      actionInputsError.value = null
      actionInputsAction.value = action
      actionInputsItems.value = [item]
      return
    }
    if (action?.type === 'clipboard') {
      const item = selectedItem.value
      if (item) await copyActionToClipboard(actionID, [item])
      return
    }
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

  async function submitSessionLaunch(input: { name: string; repository?: string; workspace?: string; agent?: string; inputs: Record<string, string> }) {
    const action = sessionLaunchAction.value
    const item = sessionLaunchItem.value
    if (!action || !item || sessionLaunchBusy.value) return
    const { inputs, ...session } = input
    sessionLaunchBusy.value = true
    sessionLaunchError.value = null
    const succeeded = await runAction(action.id, { session, inputs }, item)
    sessionLaunchBusy.value = false
    if (succeeded) cancelSessionLaunch()
    else if (!actionRerunConfirmation.value) sessionLaunchError.value = actionError.value ?? 'Could not create the session.'
  }

  function cancelActionInputs() {
    if (actionInputsBusy.value) return
    actionInputsAction.value = null
    actionInputsItems.value = []
    actionInputsError.value = null
  }

  // A clipboard action is copied rather than run, so its collected values go
  // to the render path instead of an invocation.
  async function submitActionInputs(values: Record<string, string>) {
    const action = actionInputsAction.value
    const targetItems = actionInputsItems.value
    const item = targetItems[0]
    if (!action || !item || actionInputsBusy.value) return
    actionInputsBusy.value = true
    actionInputsError.value = null
    const succeeded = action.type === 'clipboard'
      ? await copyActionToClipboard(action.id, targetItems, values)
      : await runAction(action.id, { inputs: values }, item)
    actionInputsBusy.value = false
    if (succeeded) cancelActionInputs()
    else if (!actionRerunConfirmation.value) actionInputsError.value = actionError.value ?? 'Could not run the action.'
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

  // Opens an item's canonical GitHub URL. The URL comes from the inbox item;
  // a missing one means the source did not carry it.
  async function openItemInBrowser(item: InboxItem | null) {
    const url = item?.url
    if (!url) {
      showToast('No link available for this item', { severity: 'error' })
      return
    }
    await openUrl(url)
  }

  const openSelectedInBrowser = () => openItemInBrowser(selectedItem.value)

  const clipboard = useClipboard()
  async function copyToClipboard(text: string, copiedMessage: string): Promise<void> {
    await clipboard.copy(text)
    if (clipboard.status.value === 'error') showToast('Could not copy to the clipboard', { severity: 'error' })
    else showToast(copiedMessage, { severity: 'success' })
  }

  async function copyItemLink(item: InboxItem): Promise<void> {
    if (!item.url) {
      showToast('No link available for this item', { severity: 'error' })
      return
    }
    await copyToClipboard(item.url, 'Link copied')
  }

  const copyItemContents = (item: InboxItem) => copyToClipboard(clipboardText(item), 'Contents copied')

  async function copySelectedItemContents(): Promise<void> {
    const targetItems = selectedItems.value
    if (targetItems.length === 0) return
    await copyToClipboard(targetItems.map(clipboardText).join('\n\n'), `${targetItems.length} item${targetItems.length === 1 ? '' : 's'} copied`)
  }

  async function invokeSelectionAction(actionID: string): Promise<void> {
    const action = selectionActions.value.find((candidate) => candidate.id === actionID)
    const targetItems = [...selectedItems.value]
    if (!action || targetItems.length === 0) return
    if (action.inputs?.length) {
      actionInputsError.value = null
      actionInputsAction.value = action
      actionInputsItems.value = targetItems
      return
    }
    await copyActionToClipboard(actionID, targetItems)
  }

  async function copyActionToClipboard(actionID: string, targetItems: InboxItem[], inputs: Record<string, string> = {}): Promise<boolean> {
    const itemIDs = targetItems.map((item) => item.id)
    if (itemIDs.length === 0) return false
    const key = actionKey(itemIDs[0]!, actionID)
    if (pendingActionKeys.value[key]) return false
    pendingActionKeys.value = { ...pendingActionKeys.value, [key]: true }
    actionError.value = null
    try {
      const text = await RenderClipboardAction(actionID, itemIDs, inputs)
      await copyToClipboard(text, 'Copied')
      if (clipboard.status.value !== 'error') return true
      actionError.value = 'Could not copy to the clipboard.'
      return false
    } catch (error) {
      console.warn('Unable to copy action to clipboard', error)
      const message = error instanceof Error && error.message ? error.message : 'Could not copy to the clipboard.'
      actionError.value = message
      showToast(message, { severity: 'error' })
      return false
    } finally {
      const next = { ...pendingActionKeys.value }; delete next[key]; pendingActionKeys.value = next
    }
  }

  // Row-level entry point for a configured action: actions load per selected
  // item (labels, requiresSessionInput, run cards all key off the selection),
  // so running from a row first selects that row — which also surfaces the
  // run's feedback in the detail pane.
  async function runItemAction(item: InboxItem, actionID: string): Promise<void> {
    if (selectedId.value !== item.id) await selectItem(item.id)
    await invokeAction(actionID)
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
    useWailsEvent('actions:updated', () => {
      void loadActions(selectedItem.value)
      void loadSelectionActions()
    })
    void loadProfiles()
  })

  return {
    refreshingSources,
    refreshSources,
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
    visibleArchivedItems,
    archivedExpanded,
    archivedCount,
    toggleArchivedSection,
    trashFilter,
    setTrashFilter,
    unreadCount,
    search,
    authors,
    authorFilter,
    loadError,
    selectedId,
    selectedItem,
    itemSelectionActive,
    selectedItemIDs,
    selectedItems,
    selectionActions,
    enterItemSelection,
    toggleItemSelection,
    cancelItemSelection,
    copySelectedItemContents,
    invokeSelectionAction,
    actions,
    pendingAction,
    actionError,
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
    reorderFeeds,
    reorderProfiles,
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
    markingAllRead,
    markAllRead,
    unreadInScope,
    toggleArchive,
    toggleIgnored,
    loadEvents,
    refresh,
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
  }
}
