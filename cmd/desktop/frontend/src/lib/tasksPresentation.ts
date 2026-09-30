import type {
  TaskComment,
  TaskItem,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

// Pure presentation helpers for the Tasks view: filter groups, the tree the
// two-pane view renders, checkpoint-comment parsing, cascade-delete counting,
// and status styling. Kept framework-free (no Vue) so it is unit-testable in
// isolation; TasksView/TaskTreeRow/TaskDetailPane consume it. The backend's
// ListTasks deliberately returns an unsorted flat list (store ordering
// depends on which filter branch it takes), so every ordering guarantee the
// tree exposes is owned here, not assumed from RPC order.

// ── Filters ──────────────────────────────────────────────────────────────

export type TaskFilterId = 'open' | 'active' | 'done' | 'all'

export interface TaskFilter {
  id: TaskFilterId
  label: string
}

export const TASK_FILTERS: TaskFilter[] = [
  { id: 'open', label: 'Open' },
  { id: 'active', label: 'Active' },
  { id: 'done', label: 'Done' },
  { id: 'all', label: 'All' },
]

// The view's initial filter, matching the TUI's default scope.
export const DEFAULT_TASK_FILTER: TaskFilterId = 'open'

const TERMINAL_STATUSES = new Set(['done', 'cancelled'])

function isTerminalStatus(status: string): boolean {
  return TERMINAL_STATUSES.has(status)
}

export function matchesTaskFilter(item: TaskItem, filter: TaskFilterId): boolean {
  switch (filter) {
    case 'open':
      return item.status === 'open' || item.status === 'in_progress'
    case 'active':
      return item.status === 'in_progress'
    case 'done':
      return isTerminalStatus(item.status)
    case 'all':
      return true
  }
}

// filterCounts is the per-filter badge shown in the segmented control: how
// many of the currently-loaded items each filter would match.
export function filterCounts(items: TaskItem[]): Record<TaskFilterId, number> {
  const counts: Record<TaskFilterId, number> = { open: 0, active: 0, done: 0, all: 0 }
  for (const item of items) {
    for (const filter of TASK_FILTERS) {
      if (matchesTaskFilter(item, filter.id)) counts[filter.id]++
    }
  }
  return counts
}

// ── Tree ─────────────────────────────────────────────────────────────────

export interface TaskTreeNode {
  item: TaskItem
  children: TaskTreeNode[]
  // True when this item or any descendant matches the active filter, so an
  // ancestor of a match always renders even when it doesn't match itself.
  visible: boolean
  // [done/total] over this node's descendants (self excluded), computed over
  // every descendant regardless of the active filter — a done epic's counts
  // don't change just because a child is filtered out of view.
  counts: { done: number; total: number }
}

function byCreatedAtAsc(a: TaskItem, b: TaskItem): number {
  return Date.parse(a.createdAt) - Date.parse(b.createdAt)
}

function matchesQuery(item: TaskItem, query: string): boolean {
  return query === '' || item.title.toLowerCase().includes(query) || item.id.toLowerCase().includes(query)
}

// buildTaskTree ignores the input array's order entirely: roots are re-sorted
// newest-first, children oldest-first, regardless of how items arrived. This
// guards against the store returning created_at DESC for one filter branch
// and ASC for another (see module doc). search narrows visibility further to
// title/id substring matches, riding the same ancestor-of-a-match propagation
// the status filter uses.
export function buildTaskTree(items: TaskItem[], filter: TaskFilterId, search = ''): TaskTreeNode[] {
  const query = search.trim().toLowerCase()
  const ids = new Set(items.map((item) => item.id))
  const childrenByParent = new Map<string, TaskItem[]>()
  for (const item of items) {
    const siblings = childrenByParent.get(item.parentId)
    if (siblings) siblings.push(item)
    else childrenByParent.set(item.parentId, [item])
  }

  function buildNode(item: TaskItem): TaskTreeNode {
    const children = (childrenByParent.get(item.id) ?? [])
      .slice()
      .sort(byCreatedAtAsc)
      .map(buildNode)

    const counts = children.reduce(
      (acc, child) => ({
        total: acc.total + 1 + child.counts.total,
        done: acc.done + (isTerminalStatus(child.item.status) ? 1 : 0) + child.counts.done,
      }),
      { total: 0, done: 0 },
    )

    const visible = (matchesTaskFilter(item, filter) && matchesQuery(item, query)) || children.some((child) => child.visible)

    return { item, children, visible, counts }
  }

  // A root is anything whose parentId doesn't resolve within this item set —
  // that covers true roots (parentId === '') and orphans (parent existed but
  // isn't in the current scope/list, e.g. deleted or filtered by repo).
  return items
    .filter((item) => !ids.has(item.parentId))
    .slice()
    .sort((a, b) => -byCreatedAtAsc(a, b))
    .map(buildNode)
}

// ── Checkpoint comments ──────────────────────────────────────────────────

const CHECKPOINT_PREFIX = 'CHECKPOINT:'

export function isCheckpoint(comment: TaskComment): boolean {
  return comment.message.startsWith(CHECKPOINT_PREFIX)
}

export function checkpointBody(comment: TaskComment): string {
  if (!isCheckpoint(comment)) return comment.message.trim()
  return comment.message.slice(CHECKPOINT_PREFIX.length).trim()
}

// ── Cascade ──────────────────────────────────────────────────────────────

// cascadeCount is how many non-terminal (open/in_progress) descendants an
// epic has, across every depth — the "this will also close N open tasks"
// confirm copy shown before a status change cascades.
export function cascadeCount(items: TaskItem[], epicId: string): number {
  const childrenByParent = new Map<string, TaskItem[]>()
  for (const item of items) {
    const siblings = childrenByParent.get(item.parentId)
    if (siblings) siblings.push(item)
    else childrenByParent.set(item.parentId, [item])
  }

  let count = 0
  const stack = [...(childrenByParent.get(epicId) ?? [])]
  while (stack.length > 0) {
    const item = stack.pop() as TaskItem
    if (!isTerminalStatus(item.status)) count++
    stack.push(...(childrenByParent.get(item.id) ?? []))
  }
  return count
}

// ── Status styling ───────────────────────────────────────────────────────

export interface StatusMeta {
  label: string
  classes: string
}

// Semantic-token classes only (precedent: BaseBadge.vue's tone map) — never
// raw palette colors, so the styling follows the app's light/dark theme.
export function statusMeta(status: string): StatusMeta {
  switch (status) {
    case 'in_progress':
      return { label: 'In Progress', classes: 'bg-severity-running-tint text-severity-running' }
    case 'done':
      return { label: 'Done', classes: 'bg-severity-success-tint text-severity-success' }
    case 'cancelled':
      return { label: 'Cancelled', classes: 'bg-chip text-text-4' }
    case 'open':
    default:
      return { label: 'Open', classes: 'bg-chip text-text-3' }
  }
}
