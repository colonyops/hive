import { useStorage } from '@vueuse/core'
import { computed, readonly, ref } from 'vue'
import { isGlobalTabId, isPinnedTabId, type AppTabId, type GlobalTabId } from '../lib/appTabs'
import { defineStore } from './defineStore'

const MAX_RECENT = 32

/**
 * The global tabs in the title bar, after the pinned Inbox, Code and Chats. The
 * list is the user's arrangement, so it survives a relaunch; which tab is in
 * front is the route's, so this holds no "active" of its own.
 */
export const useAppTabs = defineStore('appTabs', () => {
  const stored = useStorage<string[]>('hive.titlebar.tabs', [])
  // A hand-edited or older value must not put a tab in the strip that nothing can render.
  const tabs = computed<GlobalTabId[]>(() => [...new Set(stored.value.filter(isGlobalTabId))])

  // Most recent first. Closing the tab in front returns to the one before it.
  const recent = ref<AppTabId[]>([])

  function has(id: GlobalTabId): boolean {
    return tabs.value.includes(id)
  }

  function add(id: GlobalTabId): void {
    if (!has(id)) stored.value = [...tabs.value, id]
  }

  function remove(id: GlobalTabId): void {
    stored.value = tabs.value.filter((tab) => tab !== id)
    recent.value = recent.value.filter((tab) => tab !== id)
  }

  /** Puts `id` before `before`, or last when `before` is null. */
  function move(id: GlobalTabId, before: GlobalTabId | null): void {
    if (id === before || !has(id)) return
    const rest = tabs.value.filter((tab) => tab !== id)
    const at = before === null ? -1 : rest.indexOf(before)
    rest.splice(at === -1 ? rest.length : at, 0, id)
    stored.value = rest
  }

  function visited(id: AppTabId): void {
    recent.value = [id, ...recent.value.filter((tab) => tab !== id)].slice(0, MAX_RECENT)
  }

  /** The tab to show once `leaving` goes: the last one visited that is still in the strip. */
  function previous(leaving: AppTabId): AppTabId {
    const candidate = recent.value.find((tab) => tab !== leaving && (isPinnedTabId(tab) || has(tab)))
    return candidate ?? 'inbox'
  }

  return { tabs, recent: readonly(recent), has, add, remove, move, visited, previous }
})
