// The scoped set of glyphs a feed node may show in the sidebar tree. This is
// the single frontend source of truth for feed icons: the node editor's icon
// dropdown, the feed config validator, and the sidebar row all read from here.
// It is intentionally curated (a handful of feed-relevant icons) rather than
// exposing every available icon, and must stay in sync with the Go allow-list
// in internal/desktop/pipeline/flow/nodes_terminal.go (feedIcons).
//
// Icon components are imported statically because `~icons/lucide/<name>` is a
// build-time virtual module — the path can't be constructed dynamically — so
// each supported key maps to an eagerly imported component below.
import type { Component } from 'vue'
import IconAtSign from '~icons/lucide/at-sign'
import IconBell from '~icons/lucide/bell'
import IconBug from '~icons/lucide/bug'
import IconCircleDot from '~icons/lucide/circle-dot'
import IconClock from '~icons/lucide/clock'
import IconEye from '~icons/lucide/eye'
import IconFlag from '~icons/lucide/flag'
import IconGitBranch from '~icons/lucide/git-branch'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import IconInbox from '~icons/lucide/inbox'
import IconMessageSquare from '~icons/lucide/message-square'
import IconPackage from '~icons/lucide/package'
import IconRocket from '~icons/lucide/rocket'
import IconRss from '~icons/lucide/rss'
import IconShield from '~icons/lucide/shield'
import IconSparkles from '~icons/lucide/sparkles'
import IconStar from '~icons/lucide/star'
import IconTag from '~icons/lucide/tag'
import IconUsers from '~icons/lucide/users'
import IconZap from '~icons/lucide/zap'

export interface FeedIconOption {
  value: string
  label: string
  component: Component
}

// Order here is the order shown in the editor dropdown.
export const feedIconOptions: FeedIconOption[] = [
  { value: 'git-branch', label: 'Branch', component: IconGitBranch },
  { value: 'git-pull-request', label: 'Pull request', component: IconGitPullRequest },
  { value: 'circle-dot', label: 'Issue', component: IconCircleDot },
  { value: 'message-square', label: 'Comments', component: IconMessageSquare },
  { value: 'at-sign', label: 'Mentions', component: IconAtSign },
  { value: 'rss', label: 'Feed', component: IconRss },
  { value: 'bell', label: 'Notifications', component: IconBell },
  { value: 'eye', label: 'Watching', component: IconEye },
  { value: 'star', label: 'Starred', component: IconStar },
  { value: 'bug', label: 'Bugs', component: IconBug },
  { value: 'shield', label: 'Security', component: IconShield },
  { value: 'zap', label: 'Activity', component: IconZap },
  { value: 'sparkles', label: 'AI / generated', component: IconSparkles },
  { value: 'flag', label: 'Flagged', component: IconFlag },
  { value: 'inbox', label: 'Inbox', component: IconInbox },
  { value: 'users', label: 'Team', component: IconUsers },
  { value: 'tag', label: 'Labels', component: IconTag },
  { value: 'package', label: 'Dependencies', component: IconPackage },
  { value: 'rocket', label: 'Releases', component: IconRocket },
  { value: 'clock', label: 'Recent', component: IconClock },
]

// The glyph a feed with no configured icon falls back to — matches the
// sidebar's historical default before feeds carried an icon.
export const defaultFeedIcon = 'git-branch'

const componentByKey = new Map(feedIconOptions.map((o) => [o.value, o.component]))

/** True when key is one of the supported feed icons. */
export function isFeedIcon(key: string): boolean {
  return componentByKey.has(key)
}

/**
 * Resolves a feed's icon key to a component, falling back to the default
 * glyph for an empty or unrecognized key so the sidebar always has something
 * to render.
 */
export function feedIconComponent(key?: string): Component {
  return (key && componentByKey.get(key)) || componentByKey.get(defaultFeedIcon)!
}
