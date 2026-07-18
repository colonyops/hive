// Wire types are re-exported from the generated Wails bindings so the contract
// has one source. UI code must import them from here, never from bindings/,
// keeping binding churn contained to this module and useFeedState.
export type { Action, FeedItem, FeedSource, Profile } from '../../bindings/github.com/colonyops/hive/desktop/models'

export type SidebarSelection =
  | { type: 'all' }
  | { type: 'unread' }
  | { type: 'feed'; feedId: string }
