<script setup lang="ts">
// Tasks hub view over hc's issue tracker, laid out after Linear: the epic
// hierarchy as a list beside a detail pane, or a status board with the detail
// in a drawer over it. This owns the view mode, filters, repo scope, refresh,
// prune, row actions, and keyboard navigation; TaskDetail owns everything about
// the selection through the same useTasks() singleton.
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { onKeyStroke } from '@vueuse/core'
import IconEraser from '~icons/lucide/eraser'
import IconKanban from '~icons/lucide/square-kanban'
import IconList from '~icons/lucide/list'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import AppSelect from '@hive/ui/components/ui/AppSelect.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import DrawerSheet from '@hive/ui/components/ui/DrawerSheet.vue'
import ViewHeader from '@hive/ui/components/ui/ViewHeader.vue'
import SearchField from '@hive/ui/components/ui/SearchField.vue'
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import { useOpenModalCount } from '@hive/ui/composables/useOpenModalCount'
import type { TaskItem } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { useToasts } from '../../composables/useToasts'
import { useTasks } from '../../stores/useTasks'
import { useTerminalSessions } from '../../stores/useTerminalSessions'
import { errorText } from '../../lib/appError'
import { isEditableTarget } from '../../lib/isEditableTarget'
import {
  buildTaskTree,
  doneToggleStatus,
  filterCounts,
  groupByStatus,
  matchesTaskFilter,
  subtaskCounts,
  TASK_DETAIL_PANEL,
  TASK_FILTERS,
  type TaskBoardColumn,
  type TaskStatus,
  type TaskTreeNode,
  type TaskViewMode,
} from '../../lib/tasksPresentation'
import TaskBoard from './TaskBoard.vue'
import TaskDetail from './TaskDetail.vue'
import TaskDetailPane from './TaskDetailPane.vue'
import TaskRow from './TaskRow.vue'
import { useTaskDelete } from './useTaskDelete'
import { useTaskStatusChange } from './useTaskStatusChange'

const props = withDefaults(
  defineProps<{
    /** Lives in a session tab, which owns closing: Escape and the close button are the overlay's. */
    embedded?: boolean
    /** A tab is mounted while another shows, and its bare keys must not fire from there. */
    keysActive?: boolean
  }>(),
  { embedded: false, keysActive: true },
)
const emit = defineEmits<{ close: [] }>()

const {
  repoKey,
  filter,
  view,
  items,
  repoKeys,
  selectedId,
  loading,
  loaded,
  error,
  startLiveUpdates,
  stopLiveUpdates,
  reload,
  select,
  setRepoKey,
  setFilter,
  setView,
  isCollapsed,
  toggleCollapsed,
  pruneDryRun,
  prune,
} = useTasks()

const counts = computed(() => filterCounts(items.value))
const filterOptions = TASK_FILTERS.map((taskFilter) => ({ value: taskFilter.id, label: taskFilter.label }))
const viewOptions: { value: TaskViewMode; label: string }[] = [
  { value: 'list', label: 'List' },
  { value: 'board', label: 'Board' },
]
const search = ref('')

// TerminalMode keeps the session list loaded (it mounts once at app start),
// so this reads the singleton rather than fetching. An id with no loaded row
// — an ended session — falls back to the raw id in the row's tooltip.
const { sessions } = useTerminalSessions()
const sessionNameById = computed(() => new Map(sessions.value.map((row) => [row.id, row.name])))

const parentTitleById = computed(() => new Map(items.value.map((item) => [item.id, item.title])))
const countsById = computed(() => subtaskCounts(items.value))

// ── List ────────────────────────────────────────────────────────────────────
interface TreeRow {
  node: TaskTreeNode
  depth: number
}

// buildTaskTree already decides per-node visibility (an ancestor of a match
// always renders); this walk turns that tree into the flat, indent-annotated
// list the template renders, skipping a node's children while it's collapsed.
function flatten(nodes: TaskTreeNode[], depth: number, out: TreeRow[]): void {
  for (const node of nodes) {
    if (!node.visible) continue
    out.push({ node, depth })
    if (node.children.length && !isCollapsed(node.item.id)) flatten(node.children, depth + 1, out)
  }
}

const treeRows = computed(() => {
  const out: TreeRow[] = []
  flatten(buildTaskTree(items.value, filter.value, search.value), 0, out)
  return out
})

// ── Board ───────────────────────────────────────────────────────────────────
// The board ignores the status filter: its columns are where a card moves
// to, so every status keeps one. It renders a prefix of each column instead,
// since Done alone can hold a thousand cards.
const BOARD_PAGE = 50
const boardLimits = reactive<Partial<Record<TaskStatus, number>>>({})
const boardColumns = computed<TaskBoardColumn[]>(() =>
  groupByStatus(items.value, search.value).map((group) => ({
    status: group.status,
    items: group.items.slice(0, boardLimits[group.status] ?? BOARD_PAGE),
    total: group.items.length,
  })),
)

function showMore(status: TaskStatus): void {
  boardLimits[status] = (boardLimits[status] ?? BOARD_PAGE) + BOARD_PAGE
}

// The rows keyboard navigation walks, in screen order, whatever the layout.
const navColumns = computed<TaskItem[][]>(() => {
  if (view.value === 'board') return boardColumns.value.map((column) => column.items)
  return [treeRows.value.map((row) => row.node.item)]
})
const navIds = computed(() => navColumns.value.flat().map((item) => item.id))
const visibleCount = computed(() =>
  view.value === 'list' ? treeRows.value.length : boardColumns.value.reduce((sum, column) => sum + column.total, 0),
)

// Nothing selected after the list first loads would leave the detail pane
// permanently empty on open. Re-fires whenever selection drops back to none
// (a delete, a filter change) with rows still available — it never runs while
// something is already selected, so it can't steal an existing choice.
watch(
  [loaded, navIds],
  ([isLoaded, ids]) => {
    if (isLoaded && selectedId.value === null && ids.length) select(ids[0])
  },
  { immediate: true },
)

// Closing a task under a filter that hides closed ones drops its row. The
// selection takes the row that moved into its place, so closing several in a
// row keeps the position instead of stranding the pane on a task with no row.
watch(navIds, (ids, previous) => {
  const id = selectedId.value
  if (view.value !== 'list' || !id || !ids.length || ids.includes(id)) return
  const item = items.value.find((candidate) => candidate.id === id)
  if (!item || matchesTaskFilter(item, filter.value)) return
  const index = previous.indexOf(id)
  if (index !== -1) select(ids[Math.min(index, ids.length - 1)])
})

// A scope pointing at a repo with no items — opened from a session whose
// repo has none yet, or persisted and since pruned empty — must stay a
// visible, re-selectable choice: TaskRepoKeys() only lists repos that still
// hold items, and AppSelect renders an unmatched model value as blank.
const repoOptions = computed(() => [
  { value: '', label: 'All repositories' },
  ...repoKeys.value.map((key) => ({ value: key, label: key })),
  ...(repoKey.value && !repoKeys.value.includes(repoKey.value) ? [{ value: repoKey.value, label: repoKey.value }] : []),
])

// ── Board moves ─────────────────────────────────────────────────────────────
const { showToast } = useToasts()
const statusChange = useTaskStatusChange()
watch(statusChange.error, (message) => {
  if (message) showToast(message, { severity: 'error' })
})

const taskDelete = useTaskDelete()

// The board's detail is a modal drawer over the columns: they never reflow, and
// because a drawer registers as an open modal, the board's keys stop until it
// closes. Sync, because a vanished selection is replaced in the same flush the
// list lands in, and the drawer must close rather than show the replacement.
const peekOpen = ref(false)
watch(view, () => {
  peekOpen.value = false
})
watch(
  selectedId,
  (id) => {
    if (id === null) peekOpen.value = false
  },
  { flush: 'sync' },
)

function openPeek(id: string): void {
  select(id)
  peekOpen.value = true
}

function moveTask(item: TaskItem, status: TaskStatus): void {
  select(item.id)
  statusChange.request(item, status)
}

// ── Keyboard navigation ─────────────────────────────────────────────────────
const bodyEl = ref<HTMLElement | null>(null)
const openModalCount = useOpenModalCount()

function selectAndReveal(id: string): void {
  select(id)
  bodyEl.value?.querySelector<HTMLElement>(`[data-id="${id}"]`)?.scrollIntoView?.({ block: 'nearest' })
}

function selectedPosition(): { column: number; index: number } | null {
  const columns = navColumns.value
  for (let column = 0; column < columns.length; column++) {
    const index = columns[column].findIndex((item) => item.id === selectedId.value)
    if (index !== -1) return { column, index }
  }
  return null
}

function moveSelection(delta: 1 | -1): void {
  const ids = navIds.value
  if (!ids.length) return
  const position = selectedPosition()
  if (!position) {
    selectAndReveal(ids[0])
    return
  }
  const column = navColumns.value[position.column]
  const next = column[position.index + delta]
  if (next) selectAndReveal(next.id)
}

function moveColumn(delta: 1 | -1): void {
  const position = selectedPosition()
  if (!position) return
  const columns = navColumns.value
  for (let column = position.column + delta; column >= 0 && column < columns.length; column += delta) {
    if (!columns[column].length) continue
    selectAndReveal(columns[column][Math.min(position.index, columns[column].length - 1)].id)
    return
  }
}

function keyIsOurs(event: KeyboardEvent): boolean {
  // A stacked confirm dialog owns the keyboard; a focused input/select/select
  // popover owns its own arrow keys — AppSelect's own handler already calls
  // preventDefault() for the ones it takes, so deferring to that flag (rather
  // than special-casing the component) covers any listbox the same way.
  return props.keysActive && openModalCount.value === 0 && !isEditableTarget(event.target) && !event.defaultPrevented
}

onKeyStroke(['ArrowDown', 'ArrowUp', 'j', 'k'], (event) => {
  if (!keyIsOurs(event)) return
  event.preventDefault()
  moveSelection(event.key === 'ArrowDown' || event.key === 'j' ? 1 : -1)
})

// On the board, left and right step between columns. In the list, left
// folds and right unfolds: left on a leaf (or an already-collapsed node)
// walks up to its parent, and right on an expanded node steps into its first
// child, the common tree idiom.
onKeyStroke(['ArrowLeft', 'ArrowRight', 'h', 'l'], (event) => {
  if (!keyIsOurs(event)) return
  const left = event.key === 'ArrowLeft' || event.key === 'h'
  if (view.value === 'board') {
    event.preventDefault()
    moveColumn(left ? -1 : 1)
    return
  }
  const rows = treeRows.value
  const index = rows.findIndex((row) => row.node.item.id === selectedId.value)
  if (index === -1) return
  event.preventDefault()
  const { node, depth } = rows[index]
  const expandable = node.children.length > 0
  const collapsed = expandable && isCollapsed(node.item.id)
  if (left) {
    if (expandable && !collapsed) {
      toggleCollapsed(node.item.id)
      return
    }
    for (let i = index - 1; i >= 0; i--) {
      if (rows[i].depth < depth) {
        selectAndReveal(rows[i].node.item.id)
        return
      }
    }
  } else if (collapsed) {
    toggleCollapsed(node.item.id)
  } else if (expandable) {
    moveSelection(1)
  }
})

// Vim yank: copies the focused row's id, the same string TaskDetailPane's own
// copy button puts on the clipboard. 'y' has no default browser behaviour to
// suppress, so unlike the arrows above this never calls preventDefault(). A
// yank has no button to flip to a check, so a toast is the feedback — the
// feed's copy-link convention for anchor-less copies.
const { copy: copyToClipboard, status: yankStatus } = useClipboard()
function copyId(id: string): void {
  void copyToClipboard(id).then(() => {
    if (yankStatus.value === 'error') showToast('Could not copy to the clipboard', { severity: 'error' })
    else showToast(`Copied ${id}`, { severity: 'success' })
  })
}
onKeyStroke('y', (event) => {
  const id = selectedId.value
  if (!id || !keyIsOurs(event)) return
  copyId(id)
})

onKeyStroke('d', (event) => {
  const item = items.value.find((candidate) => candidate.id === selectedId.value)
  if (!item || !keyIsOurs(event)) return
  statusChange.request(item, doneToggleStatus(item.status))
})

// ── Prune ────────────────────────────────────────────────────────────────
// 30 days is a guess absent a documented convention: old enough that a fresh
// task never gets swept, short enough that a stale root doesn't linger
// indefinitely before anyone notices.
const PRUNE_OLDER_THAN_DAYS = 30

const pruneConfirmation = useConfirmation()
const pruneError = ref<string | null>(null)

async function requestPrune(): Promise<void> {
  pruneError.value = null
  const repo = repoKey.value
  let count: number
  try {
    count = await pruneDryRun(PRUNE_OLDER_THAN_DAYS, repo)
  } catch (err) {
    pruneError.value = errorText(err, 'Could not check what pruning would remove.')
    return
  }
  pruneConfirmation.request({
    title: 'Prune old tasks',
    description: `This removes ${count} task${count === 1 ? '' : 's'} older than ${PRUNE_OLDER_THAN_DAYS} days${repo ? ` in ${repo}` : ''}. Pruning removes everything nested under a pruned root, regardless of its own status.`,
    confirmLabel: 'Prune',
    testid: 'tasks-prune-confirm',
    onConfirm: () => prune(PRUNE_OLDER_THAN_DAYS, repo),
  })
}

useEscapeToClose(() => emit('close'), { enabled: () => !props.embedded })

onMounted(() => {
  startLiveUpdates()
})
onUnmounted(() => {
  stopLiveUpdates()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="tasks-view">
    <ViewHeader>
      <template #title>
        <span class="text-body font-semibold text-text">Tasks</span>
        <span class="font-mono text-caption text-text-4" data-testid="tasks-count"
          >{{ items.length }} {{ items.length === 1 ? 'item' : 'items' }}</span
        >
        <SegmentedControl
          :model-value="view"
          variant="compact"
          size="sm"
          :options="viewOptions"
          aria-label="Layout"
          testid="tasks-view-mode"
          class="ml-2"
          @update:model-value="setView"
        >
          <template #option="{ option }">
            <component :is="option.value === 'board' ? IconKanban : IconList" class="size-3.5" aria-hidden="true" />
            {{ option.label }}
          </template>
        </SegmentedControl>
        <div class="flex-1" />
        <IconButton
          label="Refresh"
          :icon="IconRefreshCw"
          size="lg"
          :busy="loading"
          data-testid="tasks-refresh"
          @click="reload"
        />
        <IconButton
          label="Prune old tasks"
          :icon="IconEraser"
          size="lg"
          data-testid="tasks-prune"
          @click="requestPrune"
        />
        <template v-if="!embedded">
          <span class="mx-1 h-4 w-px bg-strong" aria-hidden="true" />
          <IconButton label="Close" :icon="IconX" size="lg" data-testid="tasks-close" @click="emit('close')" />
        </template>
      </template>
    </ViewHeader>

    <div class="flex shrink-0 items-center gap-2 border-b border-row bg-pane px-4 py-2">
      <SearchField
        v-model="search"
        placeholder="Search by title or ID…"
        aria-label="Search tasks"
        testid="tasks-search"
        class="h-[31px] w-[260px]"
      />
      <div class="w-[220px]">
        <AppSelect
          :model-value="repoKey"
          :options="repoOptions"
          size="sm"
          searchable
          search-placeholder="Find a repository…"
          aria-label="Repository"
          testid="tasks-repo-select"
          @update:model-value="setRepoKey($event)"
        />
      </div>
      <div class="flex-1" />
      <SegmentedControl
        v-if="view === 'list'"
        :model-value="filter"
        variant="compact"
        :options="filterOptions"
        aria-label="Filter tasks"
        testid="tasks-filter"
        @update:model-value="setFilter"
      >
        <template #option="{ option }">
          {{ option.label }}
          <span class="font-mono text-caption text-text-4">{{ counts[option.value] }}</span>
        </template>
      </SegmentedControl>
    </div>

    <!-- A transient load failure keeps the last-seen items on screen (see
         useTasks.ts) rather than blanking the list, so this is a banner, not
         a replacement for it. -->
    <div
      v-if="error"
      class="shrink-0 border-b border-severity-error-border bg-severity-error-tint px-5 py-2 text-xs text-severity-error"
      role="alert"
      data-testid="tasks-error"
    >
      Couldn't refresh tasks — {{ error }}
    </div>
    <!-- A dry-run failure never opens the confirm dialog (there's nothing to
         confirm), so its error has nowhere to show but here. -->
    <div
      v-if="pruneError"
      class="shrink-0 border-b border-severity-error-border bg-severity-error-tint px-5 py-2 text-xs text-severity-error"
      role="alert"
      data-testid="tasks-prune-error"
    >
      Couldn't prune tasks — {{ pruneError }}
    </div>

    <div class="flex min-h-0 flex-1">
      <div ref="bodyEl" class="flex min-h-0 min-w-0 flex-1 flex-col bg-app">
        <div v-if="!loaded" class="flex h-full items-center justify-center font-mono text-xs text-text-4">
          Loading tasks…
        </div>
        <!-- A scoped-but-empty repo must say it is scoped: the generic copy
             would read as "there are no tasks anywhere" while another repo
             may hold plenty. -->
        <div
          v-else-if="!items.length && repoKey"
          class="flex h-full flex-col items-center justify-center gap-3 px-10 text-center"
          data-testid="tasks-empty"
        >
          <div class="text-body text-text-3">No tasks in {{ repoKey }}.</div>
          <button
            type="button"
            class="cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
            data-testid="tasks-empty-show-all"
            @click="setRepoKey('')"
          >
            Show all repositories
          </button>
        </div>
        <div
          v-else-if="!items.length"
          class="flex h-full flex-col items-center justify-center gap-2 px-10 text-center"
          data-testid="tasks-empty"
        >
          <div class="text-body text-text-3">No tasks yet.</div>
          <p class="text-xs text-text-4">
            Agents create tasks with
            <code class="rounded bg-chip px-1 py-0.5 font-mono text-caption text-text-3">hive hc create</code>.
          </p>
        </div>
        <div
          v-else-if="!visibleCount"
          class="flex h-full items-center justify-center font-mono text-xs text-text-4"
          data-testid="tasks-empty-filter"
        >
          {{ search.trim() ? 'No tasks match this search.' : 'No tasks match this filter.' }}
        </div>

        <TaskBoard
          v-else-if="view === 'board'"
          :columns="boardColumns"
          :selected-id="selectedId"
          :counts-by-id="countsById"
          :parent-title-by-id="parentTitleById"
          :session-name-by-id="sessionNameById"
          @select="openPeek"
          @move="moveTask"
          @show-more="showMore"
        />

        <div v-else class="hive-scroll min-h-0 flex-1 overflow-y-auto" data-testid="tasks-list">
          <TaskRow
            v-for="row in treeRows"
            :key="row.node.item.id"
            :item="row.node.item"
            :depth="row.depth"
            :expandable="row.node.children.length > 0"
            :collapsed="isCollapsed(row.node.item.id)"
            :counts="row.node.children.length ? row.node.counts : undefined"
            :selected="row.node.item.id === selectedId"
            :session-name="sessionNameById.get(row.node.item.sessionId)"
            @select="select"
            @toggle="toggleCollapsed"
            @status="statusChange.request"
            @copy="copyId"
            @delete="taskDelete.request"
          />
        </div>
      </div>

      <TaskDetailPane v-if="view === 'list'" />
    </div>

    <DrawerSheet
      v-if="view === 'board' && peekOpen"
      bare
      title="Task details"
      testid="task-detail-pane"
      v-bind="TASK_DETAIL_PANEL"
      @close="peekOpen = false"
    >
      <TaskDetail closable @close="peekOpen = false" />
    </DrawerSheet>

    <ConfirmationHost :confirmation="pruneConfirmation" />
    <ConfirmationHost :confirmation="statusChange.confirmation" />
    <ConfirmationHost :confirmation="taskDelete.confirmation" />
  </div>
</template>
