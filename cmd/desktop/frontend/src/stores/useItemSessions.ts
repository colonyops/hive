import { shallowReadonly, shallowRef } from 'vue'
import {
  ItemChats,
  ItemSessions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  ItemChatView,
  ItemSessionView,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'

// No hive install or a locked database means "nothing to show", not an error.
async function orEmpty<T>(read: () => Promise<T[] | null>): Promise<T[]> {
  try {
    return (await read()) ?? []
  } catch {
    return []
  }
}

// jobs:updated triggers a reload because session and chat launches, deletes,
// and recycles all run as jobs.
export const useItemSessions = defineStore('itemSessions', () => {
  const sessions = shallowRef<ItemSessionView[]>([])
  const chats = shallowRef<ItemChatView[]>([])

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
