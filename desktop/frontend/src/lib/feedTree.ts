// Pure logic for the sidebar's feed grouping/ordering. Kept free of Vue and
// Wails so it can be unit-tested in isolation.
//
// The persisted shape (flow.SidebarLayout, per-flow <id>.sidebar.yaml) is
// keyed by feed *node* id. The rendered shape (FeedTree) carries resolved
// FeedSummary objects. buildFeedTree reconciles the two — honoring saved
// order/folders, appending feeds the layout doesn't mention (so a newly added
// feed is never hidden) and dropping references to feeds that no longer exist.
import type { SidebarLayout as WireSidebarLayout } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/flow/models'
import type { FeedSummary, FeedTree, SidebarNode } from '../types/feed'

// The dataTransfer MIME type for a sidebar drag. Set on dragstart so a drop
// from outside the sidebar (e.g. a palette node) is never mistaken for a feed.
export const SIDEBAR_DRAG_MIME = 'application/x-hive-sidebar-item'

// DragRef identifies what is being dragged: a feed (by flow-qualified feed id)
// or a folder (by folder id).
export type DragRef = { kind: 'feed'; id: string } | { kind: 'folder'; id: string }

// DropTarget is where a drag lands:
//  - before/after an existing item (top-level, or a feed inside a folder),
//  - into a folder (append), or
//  - at the end of the top level.
export type DropTarget =
  | { kind: 'before' | 'after'; ref: DragRef }
  | { kind: 'into'; folderId: string }
  | { kind: 'top-end' }

// feedNodeId strips the "<flowId>/" prefix off a flow-qualified feed id to get
// the flow-relative node id used in the persisted layout. A feed id without
// the prefix (shouldn't happen) is returned unchanged.
export function feedNodeId(feedId: string, flowId: string): string {
  const prefix = `${flowId}/`
  return feedId.startsWith(prefix) ? feedId.slice(prefix.length) : feedId
}

// buildFeedTree resolves the saved layout against the profile's live feeds.
export function buildFeedTree(
  feeds: FeedSummary[],
  layout: WireSidebarLayout | null | undefined,
  flowId: string,
): FeedTree {
  const byNode = new Map<string, FeedSummary>()
  for (const feed of feeds) byNode.set(feedNodeId(feed.id, flowId), feed)

  const used = new Set<string>()
  const take = (nodeId: string): FeedSummary | undefined => {
    if (used.has(nodeId)) return undefined
    const feed = byNode.get(nodeId)
    if (feed) used.add(nodeId)
    return feed
  }

  const tree: FeedTree = []
  for (const item of layout?.items ?? []) {
    if (item?.folder) {
      const folderFeeds: FeedSummary[] = []
      for (const nodeId of item.folder.feeds ?? []) {
        const feed = take(nodeId)
        if (feed) folderFeeds.push(feed)
      }
      // Keep folders even when empty so a user-made folder isn't silently lost
      // after its feeds are removed — it stays as a drop target.
      tree.push({
        kind: 'folder',
        folder: { id: item.folder.id, name: item.folder.name, feeds: folderFeeds },
      })
    } else if (item?.feed) {
      const feed = take(item.feed)
      if (feed) tree.push({ kind: 'feed', feed })
    }
  }

  // Append any feeds the layout didn't place, in the flow's feed order.
  for (const feed of feeds) {
    if (!used.has(feedNodeId(feed.id, flowId))) tree.push({ kind: 'feed', feed })
  }
  return tree
}

// treeToLayout serializes a resolved tree back to the node-id-keyed persisted
// shape.
export function treeToLayout(tree: FeedTree, flowId: string): WireSidebarLayout {
  return {
    items: tree.map((node) =>
      node.kind === 'folder'
        ? {
            folder: {
              id: node.folder.id,
              name: node.folder.name,
              feeds: node.folder.feeds.map((f) => feedNodeId(f.id, flowId)),
            },
          }
        : { feed: feedNodeId(node.feed.id, flowId) },
    ),
  }
}

function cloneTree(tree: FeedTree): FeedTree {
  return tree.map((n) =>
    n.kind === 'feed'
      ? { kind: 'feed', feed: n.feed }
      : { kind: 'folder', folder: { ...n.folder, feeds: [...n.folder.feeds] } },
  )
}

// extract removes the dragged node (top-level or inside a folder) and returns
// it. Mutates the passed (already-cloned) tree's folders in place.
function extract(tree: FeedTree, drag: DragRef): { tree: FeedTree; node: SidebarNode | null } {
  const out: FeedTree = []
  let removed: SidebarNode | null = null
  for (const node of tree) {
    if (drag.kind === 'folder' && node.kind === 'folder' && node.folder.id === drag.id) {
      removed = node
      continue
    }
    if (drag.kind === 'feed' && node.kind === 'feed' && node.feed.id === drag.id) {
      removed = node
      continue
    }
    if (drag.kind === 'feed' && node.kind === 'folder') {
      const idx = node.folder.feeds.findIndex((f) => f.id === drag.id)
      if (idx !== -1) {
        removed = { kind: 'feed', feed: node.folder.feeds[idx] }
        node.folder.feeds.splice(idx, 1)
      }
    }
    out.push(node)
  }
  return { tree: out, node: removed }
}

function insert(tree: FeedTree, node: SidebarNode, target: DropTarget): FeedTree {
  if (target.kind === 'top-end') return [...tree, node]

  if (target.kind === 'into') {
    if (node.kind !== 'feed') return tree // folders don't nest
    return tree.map((n) =>
      n.kind === 'folder' && n.folder.id === target.folderId
        ? { kind: 'folder', folder: { ...n.folder, feeds: [...n.folder.feeds, node.feed] } }
        : n,
    )
  }

  const ref = target.ref
  const topIdx = tree.findIndex(
    (n) =>
      (ref.kind === 'feed' && n.kind === 'feed' && n.feed.id === ref.id) ||
      (ref.kind === 'folder' && n.kind === 'folder' && n.folder.id === ref.id),
  )
  if (topIdx !== -1) {
    const out = [...tree]
    out.splice(target.kind === 'before' ? topIdx : topIdx + 1, 0, node)
    return out
  }

  // ref is a feed nested in a folder — only meaningful when moving a feed.
  if (ref.kind === 'feed' && node.kind === 'feed') {
    return tree.map((n) => {
      if (n.kind !== 'folder') return n
      const idx = n.folder.feeds.findIndex((f) => f.id === ref.id)
      if (idx === -1) return n
      const feeds = [...n.folder.feeds]
      feeds.splice(target.kind === 'before' ? idx : idx + 1, 0, node.feed)
      return { kind: 'folder', folder: { ...n.folder, feeds } }
    })
  }

  // Unresolvable target — append rather than drop the node.
  return [...tree, node]
}

function sameRef(a: DragRef, b: DragRef): boolean {
  return a.kind === b.kind && a.id === b.id
}

// applyMove returns a new tree with `drag` moved to `target`. Invalid or no-op
// moves (dropping onto itself, nesting a folder) return the input unchanged.
export function applyMove(tree: FeedTree, drag: DragRef, target: DropTarget): FeedTree {
  if ((target.kind === 'before' || target.kind === 'after') && sameRef(drag, target.ref)) return tree
  if (drag.kind === 'folder' && target.kind === 'into') return tree

  const { tree: without, node } = extract(cloneTree(tree), drag)
  if (!node) return tree
  return insert(without, node, target)
}
