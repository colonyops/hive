// The feed UI's view types. A profile is now a flow and its feeds are the
// flow's feed nodes; items come from persisted feed_item rows.

// FeedItem is the rich item shape a feed_item row's opaque payload decodes to
// (the JSON of Go's feed.Item). feedId is the flow-qualified feed key
// ("<flowId>/<nodeId>") the row belongs to, so marking one read knows which
// feed to target.
export interface FeedItem {
  id: string
  feedId: string
  kind: string // "PR" | "Issue"
  repo: string
  num: number
  title: string
  author: string
  age: string
  unread: boolean
  reason?: string
  labels: string[]
  branch: string
  body: string
  prompt: string
  url: string
}

// FeedSummary is one feed (a flow feed node) in the sidebar: its id is the
// flow-qualified feed key.
export interface FeedSummary {
  id: string
  name: string
  count: number
  newCount: number
}

// FeedFolder is a named, collapsible group of feeds in the sidebar. It carries
// resolved FeedSummary objects (not ids) so the sidebar can render counts
// directly. Persisted (per-flow, by node id) as a flow.SidebarFolder.
export interface FeedFolder {
  id: string
  name: string
  collapsed: boolean
  feeds: FeedSummary[]
}

// SidebarNode is one entry in the sidebar's ordered FEEDS section: a bare feed
// or a folder. FeedTree is the resolved, ordered tree the SideBar renders — the
// reconciliation of a profile's live feeds with its saved flow.SidebarLayout
// (see lib/feedTree.ts).
export type SidebarNode =
  | { kind: 'feed'; feed: FeedSummary }
  | { kind: 'folder'; folder: FeedFolder }
export type FeedTree = SidebarNode[]

// Profile is a workspace in the UI — backed by a flow.
export interface Profile {
  id: string
  letter: string
  name: string
  sourceSummary: string
  totalCount: number
  unreadCount: number
  feeds: FeedSummary[]
  // tree is the resolved sidebar grouping/order for `feeds`. Absent until the
  // profile's feeds have loaded; the SideBar falls back to a flat list of
  // `feeds` when it is missing.
  tree?: FeedTree
}

// Scope only: the unread filter is an independent axis (unreadOnly in
// useFeedState). The sidebar "Unread" view is all-scope + filter on.
export type SidebarSelection =
  | { type: 'all' }
  | { type: 'feed'; feedId: string }
