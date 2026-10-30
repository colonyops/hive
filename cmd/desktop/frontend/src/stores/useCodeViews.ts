import { useStorage } from '@vueuse/core'
import { computed, readonly, ref, watch } from 'vue'
import {
  CodeViews as GetCodeViews,
  SetCodeViews as PersistCodeViews,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import type { SessionView as WireSessionView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import {
  ALL_SESSIONS_VIEW_ID,
  BUILT_IN_VIEWS,
  cloneSessionFilter,
  isBuiltInView,
  RUNNING_VIEW_ID,
  emptySessionFilter,
  sameSessionFilter,
  type SavedSessionView,
  type SessionCondition,
  type SessionFilter,
} from '../lib/sessionViews'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

type WireSessionCondition = NonNullable<WireSessionView['filter']['conditions']>[number]

// Go omits empty values and a zero day count, and settings.yaml can be edited
// by hand, so a condition can arrive without the keys its field needs. A field
// this build does not know passes through; the predicate lets it through too.
function conditionFromWire(condition: WireSessionCondition): SessionCondition {
  switch (condition.field) {
    case 'tag':
    case 'repo':
      return { ...condition, values: condition.values ?? [] } as SessionCondition
    case 'age':
      return { ...condition, days: condition.days ?? 0 } as SessionCondition
    default:
      return condition as unknown as SessionCondition
  }
}

function fromWire(views: WireSessionView[] | null): SavedSessionView[] {
  return (views ?? []).map((view) => ({
    id: view.id,
    name: view.name,
    filter: {
      match: view.filter.match === 'any' ? 'any' : 'all',
      conditions: (view.filter.conditions ?? []).map(conditionFromWire),
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

const LEGACY_RUNNING_ONLY_KEY = 'hive.terminal.sidebar.running-only'
const ACTIVE_VIEW_KEY = 'hive.code.active-view'

// The sidebar's "only running sessions" toggle became the built-in Running
// view. Someone who left it on keeps seeing what they saw.
function adoptRunningOnlyToggle(): void {
  try {
    const legacy = localStorage.getItem(LEGACY_RUNNING_ONLY_KEY)
    if (legacy === null) return
    if (legacy === 'true' && localStorage.getItem(ACTIVE_VIEW_KEY) === null) {
      localStorage.setItem(ACTIVE_VIEW_KEY, RUNNING_VIEW_ID)
    }
    localStorage.removeItem(LEGACY_RUNNING_ONLY_KEY)
  } catch {
    // Storage can be unavailable; the sidebar then opens on All sessions.
  }
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
  adoptRunningOnlyToggle()
  // Device-local, unlike the views: which view a window is on is a fact about
  // this machine. Restored because the bar always shows its chips and counts
  // what they hide, so a restored view never hides sessions silently.
  const activeViewId = useStorage(ACTIVE_VIEW_KEY, ALL_SESSIONS_VIEW_ID)

  // Built-ins come first, so a hand-written view that reuses a built-in id
  // cannot shadow one.
  const allViews = computed<readonly SavedSessionView[]>(() => [...BUILT_IN_VIEWS, ...views.value])
  const activeView = computed<SavedSessionView | null>(
    () => allViews.value.find((view) => view.id === activeViewId.value) ?? null,
  )
  const activeIsBuiltIn = computed(() => isBuiltInView(activeViewId.value))
  const savedFilter = computed<SessionFilter>(() => activeView.value?.filter ?? emptySessionFilter())

  const draft = ref<SessionFilter>(emptySessionFilter())
  // Sync, so an edit made right after a switch is not overwritten by a reset
  // that lands a tick later.
  watch(
    [activeViewId, setting.hydrated],
    () => {
      draft.value = cloneSessionFilter(savedFilter.value)
    },
    { immediate: true, flush: 'sync' },
  )
  // Only once hydrated: before then every stored id looks dangling. Falls back
  // to All when the active view is removed, or when the stored id names a view
  // that is gone by launch.
  watch(
    [views, setting.hydrated],
    () => {
      if (!setting.hydrated.value) return
      if (!activeView.value) activeViewId.value = ALL_SESSIONS_VIEW_ID
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
    if (!view || activeIsBuiltIn.value) return
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

  /** Puts the saved views in the order of `ids`; an id it does not know is skipped. */
  function reorder(ids: readonly string[]): void {
    const byId = new Map(views.value.map((v) => [v.id, v]))
    const ordered = ids.flatMap((id) => byId.get(id) ?? [])
    if (ordered.length !== views.value.length) return
    setting.set(ordered)
  }

  function remove(id: string): void {
    setting.set(views.value.filter((v) => v.id !== id))
  }

  return {
    views,
    hydrated: setting.hydrated,
    activeIsBuiltIn,
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
    reorder,
    remove,
  }
})
