import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Events, Window } from '@wailsio/runtime'
import { ActionsFor, CreateProfile, Items, Profiles } from '../../bindings/github.com/colonyops/hive/desktop/feedservice'
import type { Action, FeedItem, Profile, SidebarSelection } from '../types/feed'

export function useFeedState() {
  const profiles = ref<Profile[]>([])
  // False until the first Profiles() resolves: the app must distinguish
  // "no workspaces yet" (onboarding step 2) from "not loaded yet".
  const profilesLoaded = ref(false)
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
  const toast = ref<string | null>(null)
  const creatingProfile = ref(false)
  const createProfileError = ref<string | null>(null)
  let toastTimeout: ReturnType<typeof setTimeout> | undefined
  // Monotonic token: loadItems responses arriving out of order (slow feed
  // switch, live backend latency) must not clobber the newest request.
  let loadSeq = 0

  const activeProfile = computed(() => profiles.value.find((profile) => profile.id === activeProfileId.value) ?? null)
  const selectedItem = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
  const title = computed(() => {
    const currentSelection = selection.value
    if (currentSelection.type === 'feed') return activeProfile.value?.feeds?.find((feed) => feed.id === currentSelection.feedId)?.name ?? 'Feed'
    return unreadOnly.value ? 'Unread' : 'All items'
  })
  const countLabel = computed(() => activeProfile.value ? `${activeProfile.value.totalCount} · ${activeProfile.value.unreadCount} unread` : '')

  function showToast(message: string) {
    toast.value = message
    if (toastTimeout) clearTimeout(toastTimeout)
    toastTimeout = setTimeout(() => { toast.value = null }, 2000)
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
      profilesLoaded.value = true
    }
  }

  async function createProfile(name: string) {
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

  async function selectProfile(profileID: string) {
    activeProfileId.value = profileID
    unreadOnly.value = false
    await selectSidebar({ type: 'all' })
  }

  async function selectSidebar(nextSelection: SidebarSelection) {
    selection.value = nextSelection
    await loadItems(nextSelection.type === 'feed' ? nextSelection.feedId : '')
  }

  // The sidebar "Unread" view: all-scope with the unread filter on.
  async function selectUnreadView() {
    unreadOnly.value = true
    await selectSidebar({ type: 'all' })
  }

  async function selectItem(id: string) {
    selectedId.value = id
    await loadActions(selectedItem.value)
  }

  async function toggleUnread() {
    unreadOnly.value = !unreadOnly.value
    if (!unreadOnly.value) return
    if (selectedItem.value && !selectedItem.value.unread) {
      const firstUnread = items.value.find((item) => item.unread) ?? null
      selectedId.value = firstUnread?.id ?? null
      await loadActions(firstUnread)
    }
  }

  async function refresh() {
    await loadItems(selection.value.type === 'feed' ? selection.value.feedId : '')
  }

  function notWired() {
    showToast('Not wired up yet')
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
  // navigation happening at the same moment.
  async function onFeedUpdated(profileID: string) {
    try {
      profiles.value = (await Profiles()) ?? []
    } catch (error) {
      console.warn('Unable to refresh profiles after feed update', error)
    }
    if (profileID === activeProfileId.value) await refresh()
  }

  let unsubscribeFeed: (() => void) | undefined

  onMounted(() => {
    unsubscribeFeed = Events.On('feed:updated', (event) => {
      const profileID = Array.isArray(event.data) ? event.data[0] : event.data
      void onFeedUpdated(typeof profileID === 'string' ? profileID : '')
    })
    void loadProfiles()
  })

  onUnmounted(() => {
    unsubscribeFeed?.()
  })

  return {
    profiles,
    profilesLoaded,
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
    toast,
    creatingProfile,
    createProfileError,
    loadProfiles,
    createProfile,
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
