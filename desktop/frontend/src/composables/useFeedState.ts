import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Browser, Events, Window } from '@wailsio/runtime'
import { CreateFlow, DeleteFlow, GetFlow, GetSidebar, ListFlows, SaveSidebar } from '../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { ActionRun, ActionViews, FeedItemCounts, FeedItems, InvokeAction, MarkFeedItemRead, SessionLaunchOptions } from '../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import type { ActionRunView, SessionLaunchOptions as SessionLaunchOptionsView } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'
import { useActivity } from './useActivity'
import { buildFeedTree, treeToLayout } from '../lib/feedTree'
import type { ActionView } from '../types/action'
import type { FeedItem, FeedSummary, FeedTree, Profile, SidebarSelection } from '../types/feed'
import type { ToastInstance, ToastOptions } from '../types/toast'

// A profile IS a flow: the profiles list comes from FlowsService.ListFlows, a
// profile's sidebar feeds are the flow's feed nodes, and items come from the
// persisted feed_item rows those nodes commit (PipelineService). The old
// profiles.yaml + feed/source CRUD are gone — editing a source or feed is
// editing its node in the flow canvas.
//
// feed_item rows are written by the flow graph runtime manager — an
// app-level instance owned by pipeline/composables/useFlowsSession.ts, not
// this module. It runs every enabled flow independently; refresh() here just
// re-reads the selected profile after App.vue's "log:appended" handler has
// pumped all committed work.
export function useFeedState() {
  const profiles = ref<Profile[]>([])
  const profilesLoaded = ref(false)
  const profilesError = ref<string | null>(null)
  const activeProfileId = ref('')
  const selection = ref<SidebarSelection>({ type: 'all' })
  const unreadOnly = ref(false)
  const items = ref<FeedItem[]>([])
  const loadError = ref<string | null>(null)
  const selectedId = ref<string | null>(null)
  const actions = ref<ActionView[]>([])
  const pendingActionKeys = ref<Record<string, boolean>>({})
  const actionError = ref<string | null>(null)
  const actionRunsByItem = ref<Record<string, Record<string, ActionRunView>>>({})
  const actionRunGenerations = new Map<string, number>()
  const actionRunIDs = loadActionRunIDs()
  const pendingAction = computed(() => selectedId.value ? Object.keys(pendingActionKeys.value).find((key) => key.startsWith(`${selectedId.value}\u0000`))?.split('\u0000')[1] ?? null : null)
  const actionRuns = computed(() => selectedId.value ? actionRunsByItem.value[selectedId.value] ?? {} : {})
  const sessionLaunchAction = ref<ActionView | null>(null)
  const sessionLaunchItem = ref<FeedItem | null>(null)
  const sessionLaunchOptions = ref<SessionLaunchOptionsView | null>(null)
  const sessionLaunchBusy = ref(false)
  const sessionLaunchError = ref<string | null>(null)
  const toasts = ref<ToastInstance[]>([])
  const creatingProfile = ref(false)
  const createProfileError = ref<string | null>(null)
  const deletingProfile = ref(false)
  let nextToastId = 1
  const toastTimers = new Map<number, ReturnType<typeof setTimeout>>()
  const defaultToastDuration = 4000
  // Monotonic token: out-of-order loadItems responses must not clobber newer.
  let loadSeq = 0
  let actionLoadSeq = 0

  function actionKey(itemID: string, actionID: string): string { return `${itemID}\u0000${actionID}` }
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
  function nextActionRunGeneration(itemID: string, actionID: string): void {
    const key = actionKey(itemID, actionID)
    actionRunGenerations.set(key, (actionRunGenerations.get(key) ?? 0) + 1)
  }
  function isCurrentActionRun(itemID: string, actionID: string, commandID: number, generation: number): boolean {
    return actionRunIDs[itemID]?.[actionID] === commandID && (actionRunGenerations.get(actionKey(itemID, actionID)) ?? 0) === generation
  }
  function setActionRun(itemID: string, actionID: string, run: ActionRunView): void {
    actionRunsByItem.value = { ...actionRunsByItem.value, [itemID]: { ...(actionRunsByItem.value[itemID] ?? {}), [actionID]: run } }
    actionRunIDs[itemID] = { ...(actionRunIDs[itemID] ?? {}), [actionID]: run.commandId }
    nextActionRunGeneration(itemID, actionID)
    persistActionRunIDs()
  }
  function removeActionRunID(itemID: string, actionID: string): void {
    if (!actionRunIDs[itemID]?.[actionID]) return
    const itemRuns = { ...actionRunIDs[itemID] }; delete itemRuns[actionID]
    if (Object.keys(itemRuns).length) actionRunIDs[itemID] = itemRuns
    else delete actionRunIDs[itemID]
    nextActionRunGeneration(itemID, actionID)
    persistActionRunIDs()
  }

  const activeProfile = computed(() => profiles.value.find((p) => p.id === activeProfileId.value) ?? null)
  const selectedItem = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
  const title = computed(() => {
    const sel = selection.value
    if (sel.type === 'feed') return activeProfile.value?.feeds.find((f) => f.id === sel.feedId)?.name ?? 'Feed'
    return unreadOnly.value ? 'Unread' : 'All items'
  })

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
  function toProfileStub(flow: { id: string; name: string }): Profile {
    const name = flow.name || flow.id
    return { id: flow.id, letter: letter(name), name, sourceSummary: '', totalCount: 0, unreadCount: 0, feeds: [] }
  }

  async function loadProfiles() {
    try {
      const flows = (await ListFlows()) ?? []
      profiles.value = flows.map(toProfileStub)
      profilesLoaded.value = true
      profilesError.value = null
      const active = profiles.value.find((p) => p.id === activeProfileId.value) ?? profiles.value[0]
      if (active) await selectProfile(active.id)
      else clearActive()
    } catch (error) {
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
    try {
      profiles.value = ((await ListFlows()) ?? []).map(toProfileStub)
      if (activeProfileId.value) await loadFeeds(activeProfileId.value)
    } catch (error) {
      console.warn('Unable to refresh flows', error)
    }
  }

  // loadFeeds populates the active profile's sidebar feeds from its flow's
  // feed nodes, with per-feed counts from feed_item.
  async function loadFeeds(flowId: string) {
    try {
      const [flow, counts, sidebar] = await Promise.all([GetFlow(flowId), FeedItemCounts(flowId), GetSidebar(flowId)])
      const countByFeed = new Map((counts ?? []).map((c) => [c.feedId, c]))
      const nodes = (flow.nodes ?? []) as Array<{ id: string; type: string; name?: string }>
      const feeds: FeedSummary[] = nodes
        .filter((n) => n.type === 'feed')
        .map((n) => {
          const feedId = `${flowId}/${n.id}`
          const c = countByFeed.get(feedId)
          return { id: feedId, name: n.name || n.id, count: c?.total ?? 0, newCount: c?.unread ?? 0 }
        })
      const sourceCount = nodes.filter((n) => n.type === 'github-source').length
      const profile = profiles.value.find((p) => p.id === flowId)
      if (profile) {
        profile.feeds = feeds
        // Rebuild the sidebar tree from current feeds + saved layout on every
        // load so counts stay fresh and added/removed feeds reconcile in.
        profile.tree = buildFeedTree(feeds, sidebar, flowId)
        profile.sourceSummary = `GitHub · ${sourceCount} source${sourceCount === 1 ? '' : 's'}`
        profile.totalCount = feeds.reduce((sum, f) => sum + f.count, 0)
        profile.unreadCount = feeds.reduce((sum, f) => sum + f.newCount, 0)
      }
    } catch (error) {
      console.warn('Unable to load flow feeds', error)
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

  // ── Items (feed_item) ─────────────────────────────────────────────────────────

  function parseItem(view: { feedId: string; itemId: string; payload: any; unread: boolean }): FeedItem {
    const p = view.payload ?? {}
    return {
      id: p.id ?? view.itemId,
      feedId: view.feedId,
      kind: p.kind ?? '',
      repo: p.repo ?? '',
      num: p.num ?? 0,
      title: p.title ?? view.itemId,
      author: p.author ?? '',
      age: p.age ?? '',
      unread: view.unread,
      reason: p.reason,
      labels: p.labels ?? [],
      branch: p.branch ?? '',
      body: p.body ?? '',
      prompt: p.prompt ?? '',
      url: p.url ?? '',
    }
  }

  async function loadItems(feedID = '') {
    if (!activeProfileId.value) return
    const seq = ++loadSeq
    try {
      let views: Array<{ feedId: string; itemId: string; payload: any; unread: boolean }> = []
      if (feedID) {
        views = (await FeedItems(feedID)) ?? []
      } else {
        // "All items" aggregates across every feed of the active profile.
        const feeds = activeProfile.value?.feeds ?? []
        const lists = await Promise.all(feeds.map((f) => FeedItems(f.id)))
        views = lists.flatMap((l) => l ?? [])
      }
      if (seq !== loadSeq) return
      loadError.value = null

      // Dedupe by item id (a PR can land in two feeds); first occurrence wins.
      const seen = new Set<string>()
      const parsed: FeedItem[] = []
      for (const view of views) {
        const item = parseItem(view)
        if (seen.has(item.id)) continue
        seen.add(item.id)
        parsed.push(item)
      }
      items.value = parsed

      if (selectedId.value && parsed.some((i) => i.id === selectedId.value)) return
      const first = (unreadOnly.value ? parsed.find((i) => i.unread) : parsed[0]) ?? null
      selectedId.value = first?.id ?? null
      await loadActions(first)
    } catch (error) {
      if (seq !== loadSeq) return
      console.warn('Unable to load feed items', error)
      loadError.value = "Can't load feed items right now."
      items.value = []
      selectedId.value = null
      actions.value = []
    }
  }

  async function loadActions(item: FeedItem | null) {
    const token = ++actionLoadSeq
    if (!item) { actions.value = []; return }
    try {
      const available = (await ActionViews(item.kind)) ?? []
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

  async function selectItem(id: string) {
    selectedId.value = id
    const item = selectedItem.value
    await loadActions(item)
    if (item) await markItemRead(item)
  }

  async function markItemRead(item: FeedItem) {
    if (!item.unread) return
    try {
      await MarkFeedItemRead(item.feedId, item.id)
    } catch (error) {
      console.warn('Unable to mark item read', error)
      return
    }
    item.unread = false
    await loadFeeds(activeProfileId.value)
  }

  // ── Navigation ──────────────────────────────────────────────────────────────

  async function selectProfile(profileID: string) {
    activeProfileId.value = profileID
    unreadOnly.value = false
    await loadFeeds(profileID)
    await selectSidebar({ type: 'all' })
  }

  async function selectSidebar(nextSelection: SidebarSelection) {
    unreadOnly.value = false
    await applySelection(nextSelection)
  }

  async function selectUnreadView() {
    unreadOnly.value = true
    await applySelection({ type: 'all' })
    await reanchorToUnread()
  }

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

  async function toggleUnread() {
    unreadOnly.value = !unreadOnly.value
    if (unreadOnly.value) await reanchorToUnread()
  }

  async function refresh() {
    if (activeProfileId.value) await loadFeeds(activeProfileId.value)
    await loadItems(currentFeedId())
  }

  async function runAction(actionID: string, input: Record<string, unknown> = {}, item = selectedItem.value) {
    if (!item) return false
    const key = actionKey(item.id, actionID)
    if (pendingActionKeys.value[key]) return false
    pendingActionKeys.value = { ...pendingActionKeys.value, [key]: true }
    actionError.value = null
    try {
      const run = await InvokeAction(actionID, item, input)
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
  // button). The URL comes from the feed_item payload; a missing one means
  // the source didn't carry it.
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
  // commit into feed_item must complete BEFORE this module's refresh() reads
  // it back. Subscribing here too would race that commit and could read
  // stale feed_item rows.

  let unsubscribeFlows: (() => void) | undefined
  let unsubscribeActions: (() => void) | undefined

  onMounted(() => {
    // A flows/*.yaml change (create/delete/edit) reshapes the profiles list.
    unsubscribeFlows = Events.On('flows:updated', () => { void reloadProfilesQuietly() })
    unsubscribeActions = Events.On('actions:updated', () => { void loadActions(selectedItem.value) })
    void loadProfiles()
  })

  onUnmounted(() => {
    unsubscribeFlows?.()
    unsubscribeActions?.()
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
    pendingAction,
    actionError,
    actionRuns,
    sessionLaunchAction,
    sessionLaunchOptions,
    sessionLaunchBusy,
    sessionLaunchError,
    unreadOnly,
    title,
    toasts,
    showToast,
    dismissToast,
    clearToasts,
    creatingProfile,
    createProfileError,
    deletingProfile,
    loadProfiles,
    createProfile,
    deleteProfile,
    reorderFeeds,
    selectProfile,
    selectSidebar,
    selectUnreadView,
    selectItem,
    toggleUnread,
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
