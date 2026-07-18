// Wire types are re-exported from the generated Wails bindings so the contract
// has one source. UI code must import them from here, never from bindings/,
// keeping binding churn contained to this module and useFeedState. The Go
// types live in internal/desktop/feed; the aliases keep component-facing
// names stable across that move.
export type { Action, Item as FeedItem, Source as FeedSource, Profile } from '../../bindings/github.com/colonyops/hive/internal/desktop/feed/models'

export type SidebarSelection =
  | { type: 'all' }
  | { type: 'unread' }
  | { type: 'feed'; feedId: string }
