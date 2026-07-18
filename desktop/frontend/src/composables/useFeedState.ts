import { computed, onMounted, ref } from 'vue'
import { Window } from '@wailsio/runtime'
import { ActionsFor, Items, Profiles } from '../../bindings/github.com/colonyops/hive/desktop/feedservice'
import type { Action, FeedItem, Profile, SidebarSelection } from '../types/feed'

export function useFeedState() {
  const profiles = ref<Profile[]>([])
  const activeProfileId = ref('')
  const selection = ref<SidebarSelection>({ type: 'all' })
  const items = ref<FeedItem[]>([])
  const selectedId = ref<string | null>(null)
  const actions = ref<Action[]>([])
  const unreadOnly = ref(false)
  const toast = ref<string | null>(null)
  let toastTimeout: ReturnType<typeof setTimeout> | undefined

  const activeProfile = computed(() => profiles.value.find((profile) => profile.id === activeProfileId.value) ?? null)
  const selectedItem = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
  const title = computed(() => {
    const currentSelection = selection.value
    if (currentSelection.type === 'unread') return 'Unread'
    if (currentSelection.type === 'feed') return activeProfile.value?.feeds?.find((feed) => feed.id === currentSelection.feedId)?.name ?? 'Feed'
    return 'All items'
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
    try {
      items.value = (await Items(activeProfileId.value, feedID)) ?? []
      const first = (unreadOnly.value ? items.value.find((item) => item.unread) : items.value[0]) ?? null
      selectedId.value = first?.id ?? null
      await loadActions(first)
    } catch (error) {
      console.warn('Unable to load feed items', error)
      items.value = []
      selectedId.value = null
      actions.value = []
    }
  }

  async function selectProfile(profileID: string) {
    activeProfileId.value = profileID
    await selectSidebar({ type: 'all' })
  }

  async function selectSidebar(nextSelection: SidebarSelection) {
    selection.value = nextSelection
    unreadOnly.value = nextSelection.type === 'unread'
    await loadItems(nextSelection.type === 'feed' ? nextSelection.feedId : '')
  }

  async function selectItem(id: string) {
    selectedId.value = id
    await loadActions(selectedItem.value)
  }

  async function toggleUnread() {
    unreadOnly.value = !unreadOnly.value
    if (!unreadOnly.value) {
      // Turning the filter off while the sidebar "Unread" view is active exits
      // to "All items"; otherwise the title and highlight would still claim
      // Unread while every item renders.
      if (selection.value.type === 'unread') selection.value = { type: 'all' }
      return
    }
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

  onMounted(async () => {
    try {
      profiles.value = (await Profiles()) ?? []
      const firstProfile = profiles.value[0]
      if (firstProfile) await selectProfile(firstProfile.id)
    } catch (error) {
      console.warn('Unable to load feed profiles', error)
    }
  })

  return {
    profiles,
    activeProfile,
    activeProfileId,
    selection,
    items,
    selectedId,
    selectedItem,
    actions,
    unreadOnly,
    title,
    countLabel,
    toast,
    selectProfile,
    selectSidebar,
    selectItem,
    toggleUnread,
    refresh,
    notWired,
    hideWindow,
  }
}
