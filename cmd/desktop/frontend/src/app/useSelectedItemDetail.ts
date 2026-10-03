import { ref, watch } from 'vue'
import { useItemSessions } from '../stores/useItemSessions'
import type { FeedState } from './useAppNavigation'

/**
 * What the detail pane shows beside the selected item: its event timeline and
 * the sessions and chats started from it. App-lived rather than owned by the
 * feed view, so a trip to settings and back does not blank and refetch them.
 * Driven off the selection, so every path that moves it loads the same data.
 */
export function useSelectedItemDetail(feed: FeedState) {
  const { selectedItem } = feed
  const events = ref([] as Awaited<ReturnType<FeedState['loadEvents']>>)
  let eventsSeq = 0
  watch(
    selectedItem,
    async (item) => {
      const seq = ++eventsSeq
      events.value = []
      if (!item) return
      // A slower request for the previous item must never replace the timeline
      // of the one on screen.
      const current = () => seq === eventsSeq && selectedItem.value?.id === item.id
      try {
        const loaded = await feed.loadEvents(item.id)
        if (current()) events.value = loaded
      } catch (error) {
        if (current()) console.warn('Unable to load inbox item events', error)
      }
    },
    { immediate: true },
  )

  const { sessions, chats, load } = useItemSessions()
  watch(
    () => selectedItem.value?.id ?? null,
    (itemID) => {
      void load(itemID)
    },
    { immediate: true },
  )

  return { events, sessions, chats }
}
