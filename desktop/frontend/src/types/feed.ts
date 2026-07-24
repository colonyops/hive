// Feed presentation types. Adapter payload remains deliberately opaque until
// decoded by the GitHub presentation seam.
export type FeedSort = 'newest' | 'oldest' | 'unread'

export interface InboxItem {
  id: number
  profileId: string
  sourceKind: string
  sourceScope: string
  externalId: string
  title: string
  url: string
  payload: unknown
  revision: number
  unread: boolean
  archivedAt?: number | null
  archivedActor?: string | null
  archivedReason?: string | null
  lifecycle: string
  sourceState?: string | null
  firstSeenAt: number
  lastEventAt: number
  ignoredAt?: number | null
}

export interface InboxEvent {
  id: number
  itemId: number
  kind: string
  transition: string
  attention: string
  summary: string | null
  detail?: Record<string, unknown> | null
  createdAt: number
}

export interface FeedInboxCount {
  feedId: string
  total: number
  unread: number
  archived: number
}

export interface FeedSummary {
  id: string
  name: string
  count: number
  newCount: number
  archivedCount?: number
  icon?: string
  description?: string
}

export interface FeedFolder { id: string; name: string; feeds: FeedSummary[] }
export type SidebarNode = { kind: 'feed'; feed: FeedSummary } | { kind: 'folder'; folder: FeedFolder }
export type FeedTree = SidebarNode[]

export interface Profile {
  id: string
  letter: string
  name: string
  enabled: boolean
  sourceSummary: string
  totalCount: number
  unreadCount: number
  feeds: FeedSummary[]
  tree?: FeedTree
}

// Feeds are the only primary destinations; Trash is a de-emphasized utility
// surface for unrouted and ignored items. There is no aggregate inbox view.
export type SidebarSelection =
  | { type: 'feed'; feedId: string }
  | { type: 'trash' }
