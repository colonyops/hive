import { describe, expect, it } from 'vitest'
import type { TaskItem } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { TaskComment } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import {
  buildTaskTree,
  cascadeCount,
  checkpointBody,
  filterCounts,
  isCheckpoint,
  matchesTaskFilter,
  statusMeta,
  TASK_FILTERS,
} from '../tasksPresentation'

function task(partial: Partial<TaskItem> & { id: string }): TaskItem {
  return {
    repoKey: 'hay-kot/hive-desktop',
    epicId: '',
    parentId: '',
    sessionId: '',
    title: partial.id,
    type: 'task',
    status: 'open',
    blocked: false,
    depth: 0,
    createdAt: '2026-08-01T00:00:00.000Z',
    updatedAt: '2026-08-01T00:00:00.000Z',
    ...partial,
  }
}

function comment(partial: Partial<TaskComment> & { id: string }): TaskComment {
  return { message: '', createdAt: '2026-08-01T00:00:00.000Z', ...partial }
}

const day = (n: number) => `2026-08-${String(n).padStart(2, '0')}T00:00:00.000Z`

describe('buildTaskTree ordering', () => {
  // The backend's ListTasks returns items unsorted (store ordering depends on
  // which filter branch it takes: created_at DESC for one, ASC for another),
  // so the tree must produce the same order no matter which way the RPC
  // happened to hand items back.
  const e1 = task({ id: 'e1', type: 'epic', createdAt: day(1) })
  const e2 = task({ id: 'e2', type: 'epic', createdAt: day(3) })
  const c1 = task({ id: 'c1', parentId: 'e1', createdAt: day(2) })
  const c2 = task({ id: 'c2', parentId: 'e1', createdAt: day(4) })

  const ascendingInput = [e1, c1, e2, c2]
  const descendingInput = [c2, e2, c1, e1]

  it('sorts roots newest-first regardless of ascending input order', () => {
    const tree = buildTaskTree(ascendingInput, 'all')
    expect(tree.map((n) => n.item.id)).toEqual(['e2', 'e1'])
  })

  it('sorts roots newest-first regardless of descending input order', () => {
    const tree = buildTaskTree(descendingInput, 'all')
    expect(tree.map((n) => n.item.id)).toEqual(['e2', 'e1'])
  })

  it('sorts children oldest-first regardless of ascending input order', () => {
    const tree = buildTaskTree(ascendingInput, 'all')
    const e1Node = tree.find((n) => n.item.id === 'e1')!
    expect(e1Node.children.map((n) => n.item.id)).toEqual(['c1', 'c2'])
  })

  it('sorts children oldest-first regardless of descending input order', () => {
    const tree = buildTaskTree(descendingInput, 'all')
    const e1Node = tree.find((n) => n.item.id === 'e1')!
    expect(e1Node.children.map((n) => n.item.id)).toEqual(['c1', 'c2'])
  })
})

describe('buildTaskTree orphan promotion', () => {
  it('promotes an item whose parent is absent from the input set to a root', () => {
    const items = [
      task({ id: 'root', createdAt: day(1) }),
      // parentId references an epic that isn't in this list (deleted, or out
      // of the current repo scope) — it must still render, as a root.
      task({ id: 'orphan', parentId: 'vanished-epic', createdAt: day(2) }),
    ]
    const tree = buildTaskTree(items, 'all')
    expect(tree.map((n) => n.item.id).sort()).toEqual(['orphan', 'root'])
  })
})

describe('buildTaskTree visibility', () => {
  it('renders ancestors whose descendant matches, even when the ancestors themselves do not', () => {
    const items = [
      task({ id: 'epic', type: 'epic', status: 'done', createdAt: day(1) }),
      task({ id: 'child-done', parentId: 'epic', status: 'done', createdAt: day(2) }),
      task({ id: 'child-open', parentId: 'epic', status: 'done', createdAt: day(3) }),
      task({ id: 'grandchild-open', parentId: 'child-open', status: 'open', createdAt: day(4) }),
    ]
    const tree = buildTaskTree(items, 'open')
    const epicNode = tree[0]
    expect(epicNode.item.id).toBe('epic')
    expect(epicNode.visible).toBe(true) // ancestor of a match

    const childDone = epicNode.children.find((n) => n.item.id === 'child-done')!
    const childOpen = epicNode.children.find((n) => n.item.id === 'child-open')!
    expect(childDone.visible).toBe(false) // no match, no matching descendant
    expect(childOpen.visible).toBe(true) // ancestor of a match

    const grandchild = childOpen.children[0]
    expect(grandchild.item.id).toBe('grandchild-open')
    expect(grandchild.visible).toBe(true) // the match itself
  })
})

describe('buildTaskTree counts', () => {
  it('computes [done/total] over the unfiltered subtree even when a filter hides some children', () => {
    const items = [
      task({ id: 'epic', type: 'epic', status: 'open', createdAt: day(1) }),
      task({ id: 'done-1', parentId: 'epic', status: 'done', createdAt: day(2) }),
      task({ id: 'done-2', parentId: 'epic', status: 'cancelled', createdAt: day(3) }),
      task({ id: 'open-1', parentId: 'epic', status: 'open', createdAt: day(4) }),
      task({ id: 'grandchild', parentId: 'open-1', status: 'done', createdAt: day(5) }),
    ]

    // Filter to 'done' — this hides open-1 and epic itself from view, but the
    // counts on the epic node must still reflect the whole subtree.
    const tree = buildTaskTree(items, 'done')
    const epicNode = tree[0]
    expect(epicNode.counts).toEqual({ done: 3, total: 4 }) // done-1, done-2, grandchild + self-excluded open-1
    expect(epicNode.counts).toEqual(buildTaskTree(items, 'all')[0].counts) // same regardless of active filter
  })

  it('is zero for a leaf with no descendants', () => {
    const tree = buildTaskTree([task({ id: 'lone', createdAt: day(1) })], 'all')
    expect(tree[0].counts).toEqual({ done: 0, total: 0 })
  })
})

describe('buildTaskTree search', () => {
  const items = [
    task({ id: 'e1', type: 'epic', title: 'Epic Alpha', createdAt: day(1) }),
    task({ id: 'c1', parentId: 'e1', title: 'Fix parser', createdAt: day(2) }),
    task({ id: 't2', title: 'Write docs', createdAt: day(3) }),
  ]

  it('narrows visibility to matches while keeping ancestors of a match', () => {
    const tree = buildTaskTree(items, 'all', 'parser')
    const epic = tree.find((n) => n.item.id === 'e1')!
    expect(epic.visible).toBe(true) // ancestor of the match
    expect(epic.children[0].visible).toBe(true) // the match itself
    expect(tree.find((n) => n.item.id === 't2')!.visible).toBe(false)
  })

  it('matches case-insensitively, on ids too, and ignores surrounding whitespace', () => {
    expect(buildTaskTree(items, 'all', '  FIX  ').find((n) => n.item.id === 'e1')!.visible).toBe(true)
    expect(buildTaskTree(items, 'all', 't2').find((n) => n.item.id === 't2')!.visible).toBe(true)
  })

  it('requires filter and search to agree on the same item', () => {
    // t2 matches the query but not a 'done' filter — nothing should be visible.
    const tree = buildTaskTree(items, 'done', 'docs')
    expect(tree.every((n) => !n.visible)).toBe(true)
  })
})

describe('matchesTaskFilter and filterCounts', () => {
  const items = [
    task({ id: '1', status: 'open' }),
    task({ id: '2', status: 'in_progress' }),
    task({ id: '3', status: 'done' }),
    task({ id: '4', status: 'cancelled' }),
  ]

  it('matches open as open+in_progress, active as in_progress only, done as done+cancelled, all as everything', () => {
    expect(items.filter((i) => matchesTaskFilter(i, 'open')).map((i) => i.id)).toEqual(['1', '2'])
    expect(items.filter((i) => matchesTaskFilter(i, 'active')).map((i) => i.id)).toEqual(['2'])
    expect(items.filter((i) => matchesTaskFilter(i, 'done')).map((i) => i.id)).toEqual(['3', '4'])
    expect(items.filter((i) => matchesTaskFilter(i, 'all')).map((i) => i.id)).toEqual(['1', '2', '3', '4'])
  })

  it('counts each filter group', () => {
    expect(filterCounts(items)).toEqual({ open: 2, active: 1, done: 2, all: 4 })
  })

  it('is all-zero for an empty list', () => {
    expect(filterCounts([])).toEqual({ open: 0, active: 0, done: 0, all: 0 })
  })

  it('exposes the filter groups in segmented-control order', () => {
    expect(TASK_FILTERS.map((f) => f.id)).toEqual(['open', 'active', 'done', 'all'])
  })
})

describe('checkpoint comments', () => {
  it('recognizes a CHECKPOINT: prefixed message and strips + trims the body', () => {
    const c = comment({ id: '1', message: 'CHECKPOINT:   made progress on the seam  ' })
    expect(isCheckpoint(c)).toBe(true)
    expect(checkpointBody(c)).toBe('made progress on the seam')
  })

  it('does not recognize a message without the prefix', () => {
    const c = comment({ id: '2', message: 'just a regular update' })
    expect(isCheckpoint(c)).toBe(false)
    expect(checkpointBody(c)).toBe('just a regular update')
  })

  it('does not match the prefix as a substring mid-message', () => {
    const c = comment({ id: '3', message: 'not a CHECKPOINT: mid-string' })
    expect(isCheckpoint(c)).toBe(false)
  })
})

describe('cascadeCount', () => {
  it('counts non-terminal descendants at every depth, ignoring other subtrees', () => {
    const items = [
      task({ id: 'epic', type: 'epic', status: 'open', createdAt: day(1) }),
      task({ id: 'a', parentId: 'epic', status: 'open', createdAt: day(2) }),
      task({ id: 'b', parentId: 'epic', status: 'done', createdAt: day(3) }),
      task({ id: 'a1', parentId: 'a', status: 'in_progress', createdAt: day(4) }),
      task({ id: 'b1', parentId: 'b', status: 'cancelled', createdAt: day(5) }),
      // A sibling epic's own open descendant must not be counted.
      task({ id: 'other-epic', type: 'epic', status: 'open', createdAt: day(1) }),
      task({ id: 'other-child', parentId: 'other-epic', status: 'open', createdAt: day(2) }),
    ]
    expect(cascadeCount(items, 'epic')).toBe(2) // a (open), a1 (in_progress); b and b1 are terminal
  })

  it('is zero for an epic with no descendants', () => {
    expect(cascadeCount([task({ id: 'epic', type: 'epic', createdAt: day(1) })], 'epic')).toBe(0)
  })
})

describe('statusMeta', () => {
  it('returns a label and semantic-token classes for each status, with no raw palette colors', () => {
    for (const status of ['open', 'in_progress', 'done', 'cancelled']) {
      const meta = statusMeta(status)
      expect(meta.label).not.toBe('')
      expect(meta.classes).not.toMatch(/#[0-9a-f]{3,8}/i)
      expect(meta.classes).not.toMatch(/-(red|green|blue|amber|emerald|yellow)-\d/)
    }
  })

  it('falls back to the open styling for an unknown status', () => {
    expect(statusMeta('unknown')).toEqual(statusMeta('open'))
  })
})
