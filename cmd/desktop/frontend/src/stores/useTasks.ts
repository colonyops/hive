import { onScopeDispose, readonly, ref, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import {
  DeleteTask,
  ListTasks,
  PruneTasks,
  SetTaskStatus,
  TaskDetail as ReadTaskDetail,
  TaskRepoKeys,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/tasksservice'
import type {
  TaskDetail,
  TaskItem,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { appErrorKind } from '../lib/appError'
import { DEFAULT_TASK_FILTER, type TaskFilterId } from '../lib/tasksPresentation'
import { defineStore } from './defineStore'
import { useResource } from './useResource'
import { useWindowFocus } from './useWindowFocus'

// Polls on a 2s self-rescheduling setTimeout (not setInterval, so a slow read
// cannot pile up overlapping requests) with a generation counter that lets
// startPolling/stopPolling cut a running chain off cleanly. The hc store is a
// CLI's too — the poll is what surfaces a `hive hc` write made outside the app
// within one tick.
const POLL_INTERVAL_MS = 2000

export const useTasks = defineStore('tasks', () => {
  const repoKey = useStorage('hive.tasks.repo', '')
  const filter = useStorage<TaskFilterId>('hive.tasks.filter', DEFAULT_TASK_FILTER)
  // Epics default-expanded, so this persists the exception (which ids are
  // collapsed) rather than which are open.
  const collapsedIds = useStorage<string[]>('hive.tasks.collapsed', [])

  const repoKeys = ref<string[]>([])
  const selectedId = ref<string | null>(null)
  const detail = ref<TaskDetail | null>(null)
  // A failed read keeps the last-seen items: a transient failure must not
  // blank a list the user was already looking at.
  const list = useResource(fetchList, {
    initial: [] as TaskItem[],
    errorFallback: 'Could not load tasks.',
  })

  async function fetchList(): Promise<TaskItem[]> {
    const key = repoKey.value
    const items = (await ListTasks(key)) ?? []
    // The repo watcher has already queued a read for the new key; this one
    // must not flash the old repo's list in the meantime.
    if (key !== repoKey.value) return list.data.value
    // A selection outside the new list must not linger: the detail pane would
    // act on an item the list-derived counts (the cascade confirm's) cannot
    // see. It is dropped before the list lands so TasksView's auto-select sees
    // no selection and the new rows in the same flush.
    if (selectedId.value !== null && !items.some((item) => item.id === selectedId.value)) select(null)
    return items
  }

  let detailSequence = 0
  let repoKeysSequence = 0
  let pollGeneration = 0
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  // True whenever a view wants live updates (between startPolling and
  // stopPolling), so the window-focus reload below only fires while a Tasks
  // view is actually open.
  let pollingRequested = false

  function haltPollLoop(): void {
    ++pollGeneration
    clearTimeout(pollTimer)
    pollTimer = undefined
  }

  async function loadRepoKeys(): Promise<void> {
    const sequence = ++repoKeysSequence
    try {
      const keys = await TaskRepoKeys()
      if (sequence !== repoKeysSequence) return
      repoKeys.value = keys ?? []
    } catch (err) {
      if (sequence !== repoKeysSequence) return
      console.warn('Unable to load task repo keys', err)
    }
  }

  async function loadDetail(id: string, sequence: number): Promise<void> {
    try {
      const result = await ReadTaskDetail(id)
      if (sequence !== detailSequence || selectedId.value !== id) return
      detail.value = result
    } catch (err) {
      if (sequence !== detailSequence || selectedId.value !== id) return
      if (appErrorKind(err) === 'not_found') {
        // The item vanished externally (e.g. a CLI delete) — the selection it
        // named no longer exists.
        selectedId.value = null
        detail.value = null
        return
      }
      console.warn('Unable to load task detail', err)
    }
  }

  async function reloadDetailIfSelected(): Promise<void> {
    const id = selectedId.value
    if (!id) return
    await loadDetail(id, ++detailSequence)
  }

  async function tick(): Promise<void> {
    await Promise.all([list.reload(), reloadDetailIfSelected()])
  }

  async function poll(generation: number): Promise<void> {
    await tick()
    if (generation !== pollGeneration) return
    pollTimer = setTimeout(() => {
      void poll(generation)
    }, POLL_INTERVAL_MS)
  }

  function startPolling(): void {
    pollingRequested = true
    clearTimeout(pollTimer)
    const generation = ++pollGeneration
    void loadRepoKeys()
    void poll(generation)
  }

  function stopPolling(): void {
    pollingRequested = false
    haltPollLoop()
  }

  async function reload(): Promise<void> {
    await Promise.all([list.reload(), reloadDetailIfSelected(), loadRepoKeys()])
  }

  // A repo scope change is a server-side filter, not a client-side one (unlike
  // `filter`, which tasksPresentation applies over whatever is already
  // loaded) — so it needs its own round trip.
  watch(repoKey, () => {
    void reload()
  })

  const { focused } = useWindowFocus()
  watch(focused, (isFocused) => {
    if (isFocused && pollingRequested) void reload()
  })

  onScopeDispose(haltPollLoop)

  function setRepoKey(next: string): void {
    repoKey.value = next
  }

  function setFilter(next: TaskFilterId): void {
    filter.value = next
  }

  function select(id: string | null): void {
    selectedId.value = id
    if (id === null) {
      detail.value = null
      return
    }
    void loadDetail(id, ++detailSequence)
  }

  async function setStatus(id: string, status: string): Promise<void> {
    await SetTaskStatus(id, status)
    await reload()
  }

  async function remove(id: string): Promise<void> {
    await DeleteTask(id)
    if (selectedId.value === id) select(null)
    await reload()
  }

  /** Dry-run count for the confirm dialog — call before {@link prune}. */
  async function pruneDryRun(olderThanDays: number, repoKeyScope: string): Promise<number> {
    return await PruneTasks(olderThanDays, repoKeyScope, true)
  }

  async function prune(olderThanDays: number, repoKeyScope: string): Promise<void> {
    await PruneTasks(olderThanDays, repoKeyScope, false)
    await reload()
  }

  function isCollapsed(id: string): boolean {
    return collapsedIds.value.includes(id)
  }

  function toggleCollapsed(id: string): void {
    collapsedIds.value = isCollapsed(id)
      ? collapsedIds.value.filter((collapsed) => collapsed !== id)
      : [...collapsedIds.value, id]
  }

  return {
    repoKey: readonly(repoKey),
    filter: readonly(filter),
    collapsedIds: readonly(collapsedIds),
    items: readonly(list.data),
    repoKeys: readonly(repoKeys),
    selectedId: readonly(selectedId),
    detail: readonly(detail),
    loading: list.loading,
    loaded: list.loaded,
    error: list.error,
    setRepoKey,
    setFilter,
    startPolling,
    stopPolling,
    reload,
    select,
    setStatus,
    remove,
    pruneDryRun,
    prune,
    isCollapsed,
    toggleCollapsed,
  }
})
