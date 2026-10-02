import { shallowReadonly, shallowRef } from 'vue'
import {
  ItemChats,
  ItemSessions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  ItemChatView,
  ItemSessionView,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'

// An install with no hive behind it, or a momentarily unreadable database,
// means "nothing to show", not an error worth a pane full of red. Each list
// fails on its own, so a hive outage still shows the item's chats.
async function orEmpty<T>(read: () => Promise<T[] | null>): Promise<T[]> {
  try {
    return (await read()) ?? []
  } catch {
    return []
  }
}

// The hive sessions and agent workspace chats the selected item created. App
// drives `load` from the selection; jobs:updated re-reads, because session
// creation, chat launches, deletion and recycling all run as jobs, so that
// event is the moment the answer can have changed.
export const useItemSessions = defineStore('itemSessions', () => {
  const sessions = shallowRef<ItemSessionView[]>([])
  const chats = shallowRef<ItemChatView[]>([])

  // The item the lists on screen belong to, so a slow answer for a
  // previously-selected item cannot paint over a faster one.
  let currentItemID: number | null = null
  let loadSeq = 0

  async function load(itemID: number | null): Promise<void> {
    const seq = ++loadSeq
    currentItemID = itemID
    if (itemID === null) {
      sessions.value = []
      chats.value = []
      return
    }
    const [foundSessions, foundChats] = await Promise.all([
      orEmpty(() => ItemSessions(itemID)),
      orEmpty(() => ItemChats(itemID)),
    ])
    if (seq !== loadSeq) return
    sessions.value = foundSessions
    chats.value = foundChats
  }

  function reload(): Promise<void> {
    return load(currentItemID)
  }

  useWailsEvent('jobs:updated', () => {
    void reload()
  })

  return { sessions: shallowReadonly(sessions), chats: shallowReadonly(chats), load, reload }
})
