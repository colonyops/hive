import { ref } from 'vue'
import type { Confirmation } from '../composables/useConfirmation'
import { useNewSession } from '../composables/useNewSession'
import type { FeedState } from './useAppNavigation'

/**
 * Feed commands that the list, the palette, and the dialogs share: marking
 * read, and a new session started from the multi-item selection.
 */
export function useFeedCommands(feed: FeedState, confirmation: Confirmation) {
  const { selection, selectedItems } = feed
  const newSession = useNewSession()

  // One feed is scoped, visible in the sidebar, and what the user just asked
  // for, so it runs straight away. Trash has nothing unread to clear.
  function markSelectedFeedRead(): void {
    if (selection.value.type !== 'feed') return
    void feed.markAllRead(selection.value.feedId)
  }

  // The whole profile has no undo, so it confirms and names the count first.
  function requestMarkWorkspaceRead(): void {
    if (feed.markingAllRead.value) return
    const count = feed.unreadInScope(null)
    const scope = `every feed of ${feed.activeProfile.value?.name ?? 'this profile'}. This can't be undone.`
    confirmation.request({
      title: 'Mark all feeds as read?',
      description: count === 1 ? `Clear the unread item in ${scope}` : `Clear all ${count} unread items in ${scope}`,
      confirmLabel: 'Mark all as read',
      testid: 'mark-workspace-read-confirmation',
      onConfirm: () => feed.markAllRead(null),
    })
  }

  // A session created from the selection ends the selection; one opened any
  // other way leaves it alone.
  const creatingFromSelection = ref(false)

  async function openSelectedItemsSession(): Promise<void> {
    if (selectedItems.value.length === 0) return
    await newSession.openFromItems(selectedItems.value)
    creatingFromSelection.value = newSession.open.value
  }

  function cancelNewSession(): void {
    creatingFromSelection.value = false
    newSession.cancel()
  }

  async function submitNewSession(input: Parameters<typeof newSession.submit>[0]): Promise<void> {
    await newSession.submit(input)
    if (!newSession.open.value && creatingFromSelection.value) {
      creatingFromSelection.value = false
      feed.cancelItemSelection()
    }
  }

  return {
    markSelectedFeedRead,
    requestMarkWorkspaceRead,
    openSelectedItemsSession,
    cancelNewSession,
    submitNewSession,
  }
}
