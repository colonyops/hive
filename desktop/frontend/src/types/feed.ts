export interface FeedSource {
  id: string
  name: string
  count: number
  newCount: number
}

export interface Profile {
  id: string
  letter: string
  name: string
  sourceSummary: string
  totalCount: number
  unreadCount: number
  feeds: FeedSource[] | null
}

export interface FeedItem {
  id: string
  kind: string
  repo: string
  num: number
  title: string
  author: string
  age: string
  unread: boolean
  labels: string[] | null
  branch: string
  body: string
  prompt: string
}

export interface Action {
  id: string
  icon: string
  color: string
  title: string
  sub: string
  primary: boolean
}

export type SidebarSelection =
  | { type: 'all' }
  | { type: 'unread' }
  | { type: 'feed'; feedId: string }
