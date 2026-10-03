<script setup lang="ts">
// Tasks hub view (design modeled on ActivityView): a tree+detail split over
// hc's issue tracker. This owns the segmented filter, repo scope, refresh,
// and prune; TaskDetailPane is self-contained via the same useTasks()
// singleton and owns everything about the selection (see its own header
// comment). Reached from the titlebar, like ActivityView.
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { onKeyStroke } from '@vueuse/core'
import IconEraser from '~icons/lucide/eraser'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconX from '~icons/lucide/x'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import TaskDetailPane from './TaskDetailPane.vue'
import TaskTreeRow from './TaskTreeRow.vue'
import ViewHeader from './ui/ViewHeader.vue'
import SearchField from './ui/SearchField.vue'
import { useClipboard } from '../composables/useClipboard'
import { useConfirmation } from '../composables/useConfirmation'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import { useToasts } from '../composables/useToasts'
import { useOpenModalCount } from '../composables/useOpenModalCount'
import { useTasks } from '../stores/useTasks'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import { errorText } from '../lib/appError'
import { isEditableTarget } from '../lib/isEditableTarget'
import { buildTaskTree, filterCounts, TASK_FILTERS, type TaskTreeNode } from '../lib/tasksPresentation'
import BaseButton from './ui/BaseButton.vue'
import SegmentedControl from './ui/SegmentedControl.vue'

const emit = defineEmits<{ close: [] }>()

const {
  repoKey,
  filter,
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
  isCollapsed,
  toggleCollapsed,
  pruneDryRun,
  prune,
} = useTasks()

const counts = computed(() => filterCounts(items.value))
const filterOptions = TASK_FILTERS.map((taskFilter) => ({ value: taskFilter.id, label: taskFilter.label }))
const search = ref('')

// TerminalMode keeps the session list loaded (it mounts once at app start),
// so this reads the singleton rather than fetching. An id with no loaded row
// — an ended session — falls back to the raw id in the row's tooltip.
const { sessions } = useTerminalSessions()
const sessionNameById = computed(() => new Map(sessions.value.map((row) => [row.id, row.name])))

interface FlatRow {
  node: TaskTreeNode
  depth: number
}

// buildTaskTree already decides per-node visibility (an ancestor of a match
// always renders); this walk turns that tree into the flat, indent-annotated
// list the template renders, skipping a node's children while it's collapsed.
function flatten(nodes: TaskTreeNode[], depth: number, out: FlatRow[]): void {
  for (const node of nodes) {
    if (!node.visible) continue
    out.push({ node, depth })
    if (node.children.length && !isCollapsed(node.item.id)) flatten(node.children, depth + 1, out)
  }
}

const rows = computed(() => {
  const out: FlatRow[] = []
  flatten(buildTaskTree(items.value, filter.value, search.value), 0, out)
  return out
})

// Nothing selected after the tree first loads would leave the detail pane
// permanently empty on open. Re-fires whenever selection drops back to none
// (a delete, a filter change) with rows still available — it never runs while
// something is already selected, so it can't steal an existing choice.
watch(
  [loaded, rows],
  ([isLoaded, currentRows]) => {
    if (isLoaded && selectedId.value === null && currentRows.length) select(currentRows[0].node.item.id)
  },
  { immediate: true },
)

const repoOptions = computed<AppSelectOption[]>(() => {
  const options: AppSelectOption[] = [
    { value: '', label: 'All repositories' },
    ...repoKeys.value.map((key) => ({ value: key, label: key })),
  ]
  // A scope pointing at a repo with no items — opened from a session whose
  // repo has none yet, or persisted and since pruned empty — must stay a
  // visible, re-selectable choice: TaskRepoKeys() only lists repos that still
  // hold items, and AppSelect renders an unmatched model value as blank.
  if (repoKey.value && !repoKeys.value.includes(repoKey.value)) {
    options.push({ value: repoKey.value, label: repoKey.value })
  }
  return options
})

// ── Tree keyboard navigation ────────────────────────────────────────────────
const treeEl = ref<HTMLElement | null>(null)
const openModalCount = useOpenModalCount()

function selectAndReveal(id: string): void {
  select(id)
  treeEl.value?.querySelector<HTMLElement>(`[data-id="${id}"]`)?.scrollIntoView?.({ block: 'nearest' })
}

function moveSelection(delta: 1 | -1): void {
  const flat = rows.value
  if (!flat.length) return
  const currentIndex = flat.findIndex((row) => row.node.item.id === selectedId.value)
  const nextIndex = currentIndex === -1 ? 0 : currentIndex + delta
  if (nextIndex < 0 || nextIndex >= flat.length) return
  selectAndReveal(flat[nextIndex].node.item.id)
}

onKeyStroke(['ArrowDown', 'ArrowUp', 'j', 'k'], (event) => {
  // A stacked confirm dialog owns the keyboard; a focused input/select/select
  // popover owns its own arrow keys — AppSelect's own handler already calls
  // preventDefault() for the ones it takes, so deferring to that flag (rather
  // than special-casing the component) covers any listbox the same way.
  if (openModalCount.value > 0 || isEditableTarget(event.target) || event.defaultPrevented) return
  event.preventDefault()
  moveSelection(event.key === 'ArrowDown' || event.key === 'j' ? 1 : -1)
})

// Left folds, right unfolds — the vim h/l pairing j/k established. Left on a
// leaf (or an already-collapsed node) walks up to its parent instead, and
// right on an expanded node steps into its first child — the next visible
// row, since children render directly beneath — the common tree idiom.
onKeyStroke(['ArrowLeft', 'ArrowRight', 'h', 'l'], (event) => {
  if (openModalCount.value > 0 || isEditableTarget(event.target) || event.defaultPrevented) return
  const flat = rows.value
  const index = flat.findIndex((row) => row.node.item.id === selectedId.value)
  if (index === -1) return
  event.preventDefault()
  const { node, depth } = flat[index]
  const expandable = node.children.length > 0
  const collapsed = expandable && isCollapsed(node.item.id)
  if (event.key === 'ArrowLeft' || event.key === 'h') {
    if (expandable && !collapsed) {
      toggleCollapsed(node.item.id)
      return
    }
    for (let i = index - 1; i >= 0; i--) {
      if (flat[i].depth < depth) {
        selectAndReveal(flat[i].node.item.id)
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
const { copy: copySelectedId, status: yankStatus } = useClipboard()
const { showToast } = useToasts()
onKeyStroke('y', (event) => {
  const id = selectedId.value
  if (!id || openModalCount.value > 0 || isEditableTarget(event.target) || event.defaultPrevented) return
  void copySelectedId(id).then(() => {
    if (yankStatus.value === 'error') showToast('Could not copy to the clipboard', { severity: 'error' })
    else showToast(`Copied ${id}`, { severity: 'success' })
  })
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

useEscapeToClose(() => emit('close'))

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
        <span class="text-[13px] font-semibold text-text">Tasks</span>
        <span class="font-mono text-[11px] text-text-4"
          >{{ items.length }} {{ items.length === 1 ? 'item' : 'items' }}</span
        >
        <div class="flex-1" />
        <button
          type="button"
          class="cursor-pointer text-text-3 hover:text-text"
          aria-label="Close"
          data-testid="tasks-close"
          @click="emit('close')"
        >
          <IconX class="size-4" />
        </button>
      </template>
    </ViewHeader>

    <!-- toolbar: segmented filter + repo scope + refresh + prune -->
    <div class="flex shrink-0 flex-wrap items-center gap-2.5 border-b border-row bg-sidebar px-5 py-2.5">
      <SegmentedControl
        :model-value="filter"
        variant="compact"
        :options="filterOptions"
        aria-label="Filter tasks"
        testid="tasks-filter"
        @update:model-value="setFilter"
      >
        <template #option="{ option }">
          {{ option.label }}
          <span class="font-mono text-[10.5px] text-text-4">{{ counts[option.value] }}</span>
        </template>
      </SegmentedControl>

      <div class="w-[220px]">
        <AppSelect
          :model-value="repoKey"
          :options="repoOptions"
          size="sm"
          aria-label="Repository"
          testid="tasks-repo-select"
          @update:model-value="setRepoKey($event)"
        />
      </div>

      <SearchField
        v-model="search"
        placeholder="Filter tasks…"
        aria-label="Filter tasks"
        testid="tasks-search"
        class="w-[230px]"
      />

      <div class="flex-1" />

      <BaseButton variant="secondary" size="xs" :busy="loading" data-testid="tasks-refresh" @click="reload">
        <template #icon><IconRefreshCw class="size-3.5" /></template>Refresh
      </BaseButton>

      <BaseButton variant="secondary" size="xs" data-testid="tasks-prune" @click="requestPrune">
        <template #icon><IconEraser class="size-3.5" /></template>Prune
      </BaseButton>
    </div>

    <!-- A transient load failure keeps the last-seen items on screen (see
         useTasks.ts) rather than blanking the tree, so this is a banner, not
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

    <!-- tree + detail split -->
    <div class="flex min-h-0 flex-1">
      <div ref="treeEl" class="hive-scroll min-h-0 flex-1 overflow-y-auto bg-app" data-testid="tasks-tree">
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
          <div class="text-[13px] text-text-3">No tasks in {{ repoKey }}.</div>
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
          <div class="text-[13px] text-text-3">No tasks yet.</div>
          <p class="text-xs text-text-4">
            Agents create tasks with
            <code class="rounded bg-chip px-1 py-0.5 font-mono text-[11px] text-text-3">hive hc create</code>.
          </p>
        </div>
        <div
          v-else-if="!rows.length"
          class="flex h-full items-center justify-center font-mono text-xs text-text-4"
          data-testid="tasks-empty-filter"
        >
          {{ search.trim() ? 'No tasks match this search.' : 'No tasks match this filter.' }}
        </div>
        <template v-else>
          <TaskTreeRow
            v-for="row in rows"
            :key="row.node.item.id"
            :node="row.node"
            :depth="row.depth"
            :selected="row.node.item.id === selectedId"
            :collapsed="isCollapsed(row.node.item.id)"
            :session-name="sessionNameById.get(row.node.item.sessionId)"
            @select="select"
            @toggle="toggleCollapsed"
          />
        </template>
      </div>

      <TaskDetailPane />
    </div>

    <ConfirmationHost :confirmation="pruneConfirmation" />
  </div>
</template>
