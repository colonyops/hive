import { describe, expect, it } from 'vitest'
import type { SidebarLayout } from '../../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/flow/models'
import type { FeedSummary, FeedTree } from '../../types/feed'
import { applyMove, buildFeedTree, feedNodeId, treeToLayout } from '../feedTree'

const FLOW = 'triage'

function feed(node: string, name = node): FeedSummary {
  return { id: `${FLOW}/${node}`, name, count: 0, newCount: 0 }
}

// Convenience: the flat list of node ids a tree renders, folders flagged with
// a "dir/" prefix, for compact assertions.
function shape(tree: FeedTree): string[] {
  return tree.map((n) =>
    n.kind === 'feed'
      ? feedNodeId(n.feed.id, FLOW)
      : `${n.folder.id}[${n.folder.feeds.map((f) => feedNodeId(f.id, FLOW)).join(',')}]`,
  )
}

describe('feedNodeId', () => {
  it('strips the flow prefix', () => {
    expect(feedNodeId('triage/my-prs', 'triage')).toBe('my-prs')
  })
  it('leaves an unprefixed id unchanged', () => {
    expect(feedNodeId('my-prs', 'triage')).toBe('my-prs')
  })
})

describe('buildFeedTree', () => {
  const feeds = [feed('a'), feed('b'), feed('c')]

  it('renders a flat list in feed order when there is no layout', () => {
    expect(shape(buildFeedTree(feeds, null, FLOW))).toEqual(['a', 'b', 'c'])
  })

  it('honors saved order and folders', () => {
    const layout: SidebarLayout = {
      items: [
        { feed: 'c' },
        { folder: { id: 'work', name: 'Work', feeds: ['a', 'b'] } },
      ],
    }
    expect(shape(buildFeedTree(feeds, layout, FLOW))).toEqual(['c', 'work[a,b]'])
  })

  it('appends feeds the layout does not mention, in feed order', () => {
    const layout: SidebarLayout = { items: [{ feed: 'b' }] }
    expect(shape(buildFeedTree(feeds, layout, FLOW))).toEqual(['b', 'a', 'c'])
  })

  it('drops layout references to feeds that no longer exist', () => {
    const layout: SidebarLayout = {
      items: [
        { feed: 'gone' },
        { folder: { id: 'work', name: 'Work', feeds: ['a', 'ghost'] } },
      ],
    }
    expect(shape(buildFeedTree(feeds, layout, FLOW))).toEqual(['work[a]', 'b', 'c'])
  })

  it('keeps a folder that has become empty as a drop target', () => {
    const layout: SidebarLayout = {
      items: [{ folder: { id: 'empty', name: 'Empty', feeds: ['gone'] } }],
    }
    expect(shape(buildFeedTree(feeds, layout, FLOW))).toEqual(['empty[]', 'a', 'b', 'c'])
  })

  it('dedupes a feed referenced twice, first placement wins', () => {
    const layout: SidebarLayout = {
      items: [
        { folder: { id: 'work', name: 'Work', feeds: ['a'] } },
        { feed: 'a' },
      ],
    }
    expect(shape(buildFeedTree(feeds, layout, FLOW))).toEqual(['work[a]', 'b', 'c'])
  })

  it('round-trips through treeToLayout', () => {
    const layout: SidebarLayout = {
      items: [
        { feed: 'c' },
        { folder: { id: 'work', name: 'Work', feeds: ['a', 'b'] } },
      ],
    }
    const tree = buildFeedTree(feeds, layout, FLOW)
    expect(treeToLayout(tree, FLOW)).toEqual(layout)
  })
})

describe('applyMove', () => {
  const feeds = [feed('a'), feed('b'), feed('c'), feed('d')]
  const withFolder: SidebarLayout = {
    items: [
      { feed: 'a' },
      { folder: { id: 'work', name: 'Work', feeds: ['b', 'c'] } },
      { feed: 'd' },
    ],
  }
  const tree = () => buildFeedTree(feeds, withFolder, FLOW)

  it('reorders a top-level feed before another', () => {
    const out = applyMove(tree(), { kind: 'feed', id: `${FLOW}/d` }, { kind: 'before', ref: { kind: 'feed', id: `${FLOW}/a` } })
    expect(shape(out)).toEqual(['d', 'a', 'work[b,c]'])
  })

  it('moves a feed into a folder', () => {
    const out = applyMove(tree(), { kind: 'feed', id: `${FLOW}/a` }, { kind: 'into', folderId: 'work' })
    expect(shape(out)).toEqual(['work[b,c,a]', 'd'])
  })

  it('moves a feed out of a folder to the top level', () => {
    const out = applyMove(tree(), { kind: 'feed', id: `${FLOW}/b` }, { kind: 'after', ref: { kind: 'feed', id: `${FLOW}/d` } })
    expect(shape(out)).toEqual(['a', 'work[c]', 'd', 'b'])
  })

  it('reorders feeds within a folder', () => {
    const out = applyMove(tree(), { kind: 'feed', id: `${FLOW}/c` }, { kind: 'before', ref: { kind: 'feed', id: `${FLOW}/b` } })
    expect(shape(out)).toEqual(['a', 'work[c,b]', 'd'])
  })

  it('reorders folders among top-level items', () => {
    const out = applyMove(tree(), { kind: 'folder', id: 'work' }, { kind: 'before', ref: { kind: 'feed', id: `${FLOW}/a` } })
    expect(shape(out)).toEqual(['work[b,c]', 'a', 'd'])
  })

  it('appends to the top level end', () => {
    const out = applyMove(tree(), { kind: 'feed', id: `${FLOW}/a` }, { kind: 'top-end' })
    expect(shape(out)).toEqual(['work[b,c]', 'd', 'a'])
  })

  it('refuses to nest a folder inside a folder (no-op)', () => {
    const out = applyMove(tree(), { kind: 'folder', id: 'work' }, { kind: 'into', folderId: 'work' })
    expect(shape(out)).toEqual(['a', 'work[b,c]', 'd'])
  })

  it('is a no-op when dropped onto itself', () => {
    const out = applyMove(tree(), { kind: 'feed', id: `${FLOW}/a` }, { kind: 'before', ref: { kind: 'feed', id: `${FLOW}/a` } })
    expect(shape(out)).toEqual(['a', 'work[b,c]', 'd'])
  })

  it('is a no-op when the dragged item is unknown', () => {
    const out = applyMove(tree(), { kind: 'feed', id: `${FLOW}/nope` }, { kind: 'top-end' })
    expect(shape(out)).toEqual(['a', 'work[b,c]', 'd'])
  })

  it('does not mutate the input tree', () => {
    const original = tree()
    const snapshot = shape(original)
    applyMove(original, { kind: 'feed', id: `${FLOW}/a` }, { kind: 'into', folderId: 'work' })
    expect(shape(original)).toEqual(snapshot)
  })
})
