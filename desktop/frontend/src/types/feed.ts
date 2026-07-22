// Inbox presentation types. Adapter payload remains deliberately opaque until
// decoded by the GitHub presentation seam.
export type InboxView = 'inbox' | 'open' | 'archive' | 'all' | 'unfiled'

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
}

export interface InboxEvent {
  id: number
  itemId: number
  kind: string
  transition: string
  attention: string
  summary: string | null
  createdAt: number
}

export interface FeedInboxCount {
  feedId: string
  total: number
  unread: number
}

export interface FeedSummary {
  id: string
  name: string
  count: number
  newCount: number
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
  sourceSummary: string
  totalCount: number
  unreadCount: number
  feeds: FeedSummary[]
  tree?: FeedTree
}

export type SidebarSelection =
  | { type: 'view'; view: InboxView }
  | { type: 'feed'; feedId: string }
