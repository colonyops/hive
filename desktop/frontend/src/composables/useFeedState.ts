import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Browser, Events, Window } from '@wailsio/runtime'
import { CreateFlow, DeleteFlow, GetFlow, ListFlows } from '../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { ActionViews, FeedItemCounts, FeedItems, InvokeAction, MarkFeedItemRead } from '../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import type { ActionView } from '../types/action'
import type { FeedItem, FeedSummary, Profile, SidebarSelection } from '../types/feed'
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
  const toasts = ref<ToastInstance[]>([])
  const creatingProfile = ref(false)
  const createProfileError = ref<string | null>(null)
  const deletingProfile = ref(false)
  let nextToastId = 1
  const toastTimers = new Map<number, ReturnType<typeof setTimeout>>()
  const defaultToastDuration = 4000
  // Monotonic token: out-of-order loadItems responses must not clobber newer.
  let loadSeq = 0

  const activeProfile = computed(() => profiles.value.find((p) => p.id === activeProfileId.value) ?? null)
  const selectedItem = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
  const title = computed(() => {
    const sel = selection.value
    if (sel.type === 'feed') return activeProfile.value?.feeds.find((f) => f.id === sel.feedId)?.name ?? 'Feed'
    return unreadOnly.value ? 'Unread' : 'All items'
  })

  // ── Toasts ──────────────────────────────────────────────────────────────────

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
      const [flow, counts] = await Promise.all([GetFlow(flowId), FeedItemCounts(flowId)])
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
        profile.sourceSummary = `GitHub · ${sourceCount} source${sourceCount === 1 ? '' : 's'}`
        profile.totalCount = feeds.reduce((sum, f) => sum + f.count, 0)
        profile.unreadCount = feeds.reduce((sum, f) => sum + f.newCount, 0)
      }
    } catch (error) {
      console.warn('Unable to load flow feeds', error)
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
    deletingProfile.value = true
    try {
      await DeleteFlow(profileID)
    } catch (error) {
      console.warn('Unable to delete flow', error)
      showToast(error instanceof Error && error.message ? error.message : 'Could not delete the profile.', { severity: 'error' })
      return false
    } finally {
      deletingProfile.value = false
    }
    showToast('Profile deleted', { severity: 'success' })
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
    if (!item) {
      actions.value = []
      return
    }
    try {
      actions.value = (await ActionViews(item.kind)) ?? []
    } catch (error) {
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

  async function invokeAction(actionID: string) {
    const item = selectedItem.value
    if (!item) return
    try {
      await InvokeAction(actionID, item)
      const label = actions.value.find((action) => action.id === actionID)?.label ?? actionID
      showToast(`${label} started`, { severity: 'success' })
    } catch (error) {
      console.warn('Unable to invoke action', error)
      showToast(error instanceof Error && error.message ? error.message : 'Could not run the action.', { severity: 'error' })
    }
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
    selectProfile,
    selectSidebar,
    selectUnreadView,
    selectItem,
    toggleUnread,
    refresh,
    invokeAction,
    notWired,
    openUrl,
    openSelectedInBrowser,
    hideWindow,
  }
}
