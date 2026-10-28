import { useStorage } from '@vueuse/core'
import { computed, readonly, ref, watch } from 'vue'
import {
  CodeViews as GetCodeViews,
  SetCodeViews as PersistCodeViews,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import type { SessionView as WireSessionView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import {
  ALL_SESSIONS_VIEW_ID,
  cloneSessionFilter,
  emptySessionFilter,
  sameSessionFilter,
  type SavedSessionView,
  type SessionCondition,
  type SessionFilter,
} from '../lib/sessionViews'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

// The wire carries conditions as loose records so a field a newer build wrote
// survives the round trip; the predicate passes one it does not know.
function fromWire(views: WireSessionView[] | null): SavedSessionView[] {
  return (views ?? []).map((view) => ({
    id: view.id,
    name: view.name,
    filter: {
      match: view.filter.match === 'any' ? 'any' : 'all',
      conditions: (view.filter.conditions ?? []) as unknown as SessionCondition[],
    },
  }))
}

function toWire(views: readonly SavedSessionView[]): WireSessionView[] {
  return views.map((view) => ({
    id: view.id,
    name: view.name,
    filter: cloneSessionFilter(view.filter) as unknown as WireSessionView['filter'],
  }))
}

/**
 * The Code sidebar's saved views and the filter on screen. The filter is a
 * draft over the active view: editing it does not touch the saved view until
 * `save`, which is what lets the bar offer Save and Reset the way Linear does.
 */
export const useCodeViews = defineStore('codeViews', () => {
  const setting = usePersistedSetting<SavedSessionView[]>({
    initial: [],
    read: async () => fromWire(await GetCodeViews()),
    write: (next) => PersistCodeViews(toWire(next)),
    label: 'the Code views',
  })
  const views = setting.value
  // Device-local, unlike the views: which view a window is on is a fact about
  // this machine. Restored because the bar always names it, so a restored view
  // never hides sessions silently.
  const activeViewId = useStorage('hive.code.active-view', ALL_SESSIONS_VIEW_ID)

  const activeView = computed<SavedSessionView | null>(
    () => views.value.find((view) => view.id === activeViewId.value) ?? null,
  )
  const savedFilter = computed<SessionFilter>(() => activeView.value?.filter ?? emptySessionFilter())

  const draft = ref<SessionFilter>(emptySessionFilter())
  watch(
    [activeViewId, setting.hydrated],
    () => {
      draft.value = cloneSessionFilter(savedFilter.value)
    },
    { immediate: true },
  )
  // Only once hydrated: before then every stored id looks dangling. A view
  // deleted elsewhere (another window, a synced settings.yaml) falls back to All.
  watch(
    [views, setting.hydrated],
    () => {
      if (!setting.hydrated.value) return
      if (activeViewId.value !== ALL_SESSIONS_VIEW_ID && !activeView.value) activeViewId.value = ALL_SESSIONS_VIEW_ID
    },
    { immediate: true },
  )

  const dirty = computed(() => !sameSessionFilter(draft.value, savedFilter.value))

  function select(id: string): void {
    activeViewId.value = id
    draft.value = cloneSessionFilter(savedFilter.value)
  }

  function addCondition(condition: SessionCondition): void {
    draft.value = { ...draft.value, conditions: [...draft.value.conditions, condition] }
  }

  function updateCondition(index: number, condition: SessionCondition): void {
    const conditions = draft.value.conditions.slice()
    conditions[index] = condition
    draft.value = { ...draft.value, conditions }
  }

  function removeCondition(index: number): void {
    draft.value = { ...draft.value, conditions: draft.value.conditions.filter((_, i) => i !== index) }
  }

  function setMatch(match: SessionFilter['match']): void {
    draft.value = { ...draft.value, match }
  }

  function reset(): void {
    draft.value = cloneSessionFilter(savedFilter.value)
  }

  function save(): void {
    const view = activeView.value
    if (!view) return
    setting.set(views.value.map((v) => (v.id === view.id ? { ...v, filter: cloneSessionFilter(draft.value) } : v)))
  }

  function saveAs(name: string): void {
    const view: SavedSessionView = { id: crypto.randomUUID(), name, filter: cloneSessionFilter(draft.value) }
    setting.set([...views.value, view])
    activeViewId.value = view.id
  }

  function rename(id: string, name: string): void {
    setting.set(views.value.map((v) => (v.id === id ? { ...v, name } : v)))
  }

  function remove(id: string): void {
    setting.set(views.value.filter((v) => v.id !== id))
  }

  return {
    views,
    hydrated: setting.hydrated,
    activeViewId: readonly(activeViewId),
    activeView,
    draft: readonly(draft),
    dirty,
    select,
    addCondition,
    updateCondition,
    removeCondition,
    setMatch,
    reset,
    save,
    saveAs,
    rename,
    remove,
  }
})
