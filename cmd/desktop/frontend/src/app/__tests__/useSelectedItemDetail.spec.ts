import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import type { InboxItem } from '../../types/feed'
import type { FeedState } from '../useAppNavigation'

const load = vi.hoisted(() => vi.fn<(id: number | null) => Promise<void>>())
vi.mock('../../stores/useItemSessions', () => ({
  useItemSessions: () => ({ sessions: [], chats: [], load }),
}))

import { useSelectedItemDetail } from '../useSelectedItemDetail'

describe('useSelectedItemDetail', () => {
  it('keeps the timeline of the item on screen when an older request lands last', async () => {
    const resolvers = new Map<number, (events: unknown[]) => void>()
    const feed = {
      selectedItem: ref<InboxItem | null>(null),
      loadEvents: vi.fn((id: number) => new Promise((resolve) => resolvers.set(id, resolve))),
    }
    const detail = useSelectedItemDetail(feed as unknown as FeedState)

    feed.selectedItem.value = { id: 1 } as InboxItem
    await flushPromises()
    feed.selectedItem.value = { id: 2 } as InboxItem
    await flushPromises()

    resolvers.get(2)?.([{ kind: 'second' }])
    resolvers.get(1)?.([{ kind: 'first' }])
    await flushPromises()

    expect(detail.events.value).toEqual([{ kind: 'second' }])
    expect(load.mock.calls.map(([id]) => id)).toEqual([null, 1, 2])
  })
})
