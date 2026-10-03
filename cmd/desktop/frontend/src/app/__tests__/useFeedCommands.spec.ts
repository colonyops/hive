import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useConfirmation } from '../../composables/useConfirmation'
import type { InboxItem, SidebarSelection } from '../../types/feed'
import type { FeedState } from '../useAppNavigation'

const newSession = vi.hoisted(() => ({
  open: { value: false },
  openFromItems: vi.fn(),
  cancel: vi.fn(),
  submit: vi.fn(),
}))
vi.mock('../../composables/useNewSession', () => ({ useNewSession: () => newSession }))

import { useFeedCommands } from '../useFeedCommands'

function fakeFeed() {
  return {
    selection: ref<SidebarSelection>({ type: 'feed', feedId: 'inbox' }),
    selectedItems: ref([{ id: 1 }, { id: 2 }] as InboxItem[]),
    activeProfile: ref({ name: 'Work' }),
    markingAllRead: ref(false),
    markAllRead: vi.fn().mockResolvedValue(undefined),
    unreadInScope: vi.fn().mockReturnValue(3),
    cancelItemSelection: vi.fn(),
  }
}

type SessionInput = Parameters<ReturnType<typeof useFeedCommands>['submitNewSession']>[0]

let feed: ReturnType<typeof fakeFeed>
let confirmation: ReturnType<typeof useConfirmation>
let commands: ReturnType<typeof useFeedCommands>

beforeEach(() => {
  vi.clearAllMocks()
  newSession.open.value = false
  newSession.openFromItems.mockImplementation(() => {
    newSession.open.value = true
  })
  newSession.submit.mockImplementation(() => {
    newSession.open.value = false
  })
  feed = fakeFeed()
  confirmation = useConfirmation()
  commands = useFeedCommands(feed as unknown as FeedState, confirmation)
})

describe('useFeedCommands', () => {
  it('marks the selected feed read straight away and does nothing in Trash', () => {
    commands.markSelectedFeedRead()
    expect(feed.markAllRead).toHaveBeenCalledWith('inbox')

    feed.markAllRead.mockClear()
    feed.selection.value = { type: 'trash' }
    commands.markSelectedFeedRead()
    expect(feed.markAllRead).not.toHaveBeenCalled()
  })

  it('confirms marking the whole profile read and names the count', async () => {
    commands.requestMarkWorkspaceRead()
    expect(confirmation.options.value?.description).toContain('Clear all 3 unread items in every feed of Work')
    expect(feed.markAllRead).not.toHaveBeenCalled()

    await confirmation.confirm()
    expect(feed.markAllRead).toHaveBeenCalledWith(null)
  })

  it('ends the item selection once a session created from it is submitted', async () => {
    await commands.openSelectedItemsSession()
    expect(newSession.openFromItems).toHaveBeenCalledWith(feed.selectedItems.value)

    await commands.submitNewSession({} as SessionInput)
    expect(feed.cancelItemSelection).toHaveBeenCalledOnce()
  })

  it('keeps the selection when the dialog was cancelled first', async () => {
    await commands.openSelectedItemsSession()
    commands.cancelNewSession()
    expect(newSession.cancel).toHaveBeenCalledOnce()

    newSession.open.value = true
    await commands.submitNewSession({} as SessionInput)
    expect(feed.cancelItemSelection).not.toHaveBeenCalled()
  })

  it('keeps the selection while a failed submit leaves the dialog open', async () => {
    await commands.openSelectedItemsSession()
    newSession.submit.mockImplementation(() => {})

    await commands.submitNewSession({} as SessionInput)
    expect(feed.cancelItemSelection).not.toHaveBeenCalled()
  })
})
