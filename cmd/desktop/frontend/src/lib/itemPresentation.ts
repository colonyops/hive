// Presentation adapter registry: the seam between an inbox item's
// provider-neutral payload and the feed UI. Canonical projections (kind,
// container, byline, snippet, search, clipboard) are plain module
// functions — every sourceKind gets identical semantics, so no adapter can
// quietly fork search or clipboard behavior. Only the genuinely
// provider-variant surface (sourceLabel, the brand/glyph mark, the
// actions-footer context line) lives in the sourceKind-keyed registry below.
import type { Component } from 'vue'
import GithubMark from '../components/marks/GithubMark.vue'
import GiteaMark from '../components/marks/GiteaMark.vue'
import PostHogMark from '../components/marks/PostHogMark.vue'
import grafanaLogo from '../assets/integrations/grafana.svg'
import { defaultExecSourceIcon, defaultRssSourceIcon, defaultWebhookSourceIcon, feedIconComponent } from './feedIcons'
import * as execSourceNode from '../pipeline/nodes/sources.exec/config'
import * as giteaSourceNode from '../pipeline/nodes/sources.gitea/config'
import * as githubSourceNode from '../pipeline/nodes/sources.github/config'
import * as grafanaMetricsSourceNode from '../pipeline/nodes/sources.grafana_metrics/config'
import * as grafanaAlertsSourceNode from '../pipeline/nodes/sources.grafana_alerts/config'
import * as grafanaIRMAlertsSourceNode from '../pipeline/nodes/sources.grafana_irm_alerts/config'
import * as posthogErrorsSourceNode from '../pipeline/nodes/sources.posthog_errors/config'
import * as posthogAlertsSourceNode from '../pipeline/nodes/sources.posthog_alerts/config'
import * as rssSourceNode from '../pipeline/nodes/sources.rss/config'
import * as webhookSourceNode from '../pipeline/nodes/sources.webhook/config'
import IconActivity from '~icons/lucide/activity'
import IconCircleDot from '~icons/lucide/circle-dot'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import IconInbox from '~icons/lucide/inbox'
import type { InboxItem } from '../types/feed'

// ── Canonical payload decode ────────────────────────────────────────────────

/** Canonical payload projection (the blessed inbox-item contract keys).
 *  Every field is typeof-guarded, so any payload shape — a non-object
 *  payload, a mistyped field — degrades to empty fields instead of
 *  breaking. Applies to every sourceKind; the contract is provider-neutral. */
export interface CanonicalPayload {
  kind: string
  repo: string
  num: number
  author: string
  body: string
  url: string
  labels: string[]
  state: string
}

export type PullRequestCI = 'none' | 'passing' | 'pending' | 'failing'
export type PullRequestReview = 'open' | 'draft' | 'approved' | 'changes_requested' | 'review_required'

export interface PullRequestMetadata {
  ci: PullRequestCI | null
  review: PullRequestReview | null
  additions: number | null
  deletions: number | null
}

export function canonicalPayload(item: InboxItem): CanonicalPayload {
  if (!item.payload || typeof item.payload !== 'object') {
    return { kind: '', repo: '', num: 0, author: '', body: '', url: item.url, labels: [], state: '' }
  }
  const value = item.payload as Record<string, unknown>
  const string = (key: string): string => (typeof value[key] === 'string' ? value[key] as string : '')
  return {
    kind: string('kind'),
    repo: string('repo'),
    num: typeof value.num === 'number' ? value.num : 0,
    author: string('author'),
    body: string('body'),
    url: string('url') || item.url,
    labels: Array.isArray(value.labels) ? value.labels.filter((label): label is string => typeof label === 'string') : [],
    state: string('state'),
  }
}

export function pullRequestMetadata(item: InboxItem): PullRequestMetadata | null {
  if (kind(item) !== 'PR' || !item.payload || typeof item.payload !== 'object') return null
  const value = item.payload as Record<string, unknown>
  const ciStates = new Set<PullRequestCI>(['none', 'passing', 'pending', 'failing'])
  const reviewStates = new Set<PullRequestReview>(['open', 'draft', 'approved', 'changes_requested', 'review_required'])
  const ci = typeof value.ci === 'string' && ciStates.has(value.ci as PullRequestCI) ? value.ci as PullRequestCI : null
  const review = typeof value.review === 'string' && reviewStates.has(value.review as PullRequestReview) ? value.review as PullRequestReview : null
  const count = (key: string): number | null => {
    const entry = value[key]
    return typeof entry === 'number' && Number.isFinite(entry) && entry >= 0 ? entry : null
  }
  const metadata = { ci, review, additions: count('additions'), deletions: count('deletions') }
  return Object.values(metadata).every((entry) => entry == null) ? null : metadata
}

// ── Canonical projections ───────────────────────────────────────────────────
// Deliberately module functions, not adapter members: every provider gets
// identical kind styling, container/byline lines, snippets, search, and
// clipboard semantics.

export type KindStyle = 'pr' | 'issue' | 'neutral'

/** The kind an item carries when its payload declares none. Every item has a
 *  kind so every item is automatable: `applies_to: [Item]` targets exactly
 *  the untyped ones, and they show up in the actions editor's autocomplete
 *  like any other kind. Must stay in sync with Go's DefaultItemKind
 *  (internal/app/dispatch/action_item.go) — the action gate matches
 *  against the same value. See docs/decisions/2026-07-24-canonical-item-contract.md. */
export const DEFAULT_ITEM_KIND = 'Item'

/** Canonical kind — what applies_to matches against. Never empty:
 *  canonicalPayload keeps the raw decode (so "was a kind sent?" stays
 *  answerable), and this projection trims and applies DEFAULT_ITEM_KIND.
 *  Trimming mirrors Go's canonicalFields, so both sides agree on which
 *  items are untyped. */
export function kind(item: InboxItem): string {
  return canonicalPayload(item).kind.trim() || DEFAULT_ITEM_KIND
}

/** Human label for the kind pill/search: known kinds get a friendly name,
 *  anything else echoes the kind (including the default). */
export function kindLabel(item: InboxItem): string {
  const raw = kind(item)
  if (raw === 'PR') return 'Pull Request'
  if (raw === 'Issue') return 'Issue'
  return raw
}

/** Kind pill styling. PR/Issue styling is payload-driven, not
 *  provider-driven — a webhook item with kind "PR" gets PR styling too. */
export function kindStyle(item: InboxItem): KindStyle {
  const raw = kind(item)
  if (raw === 'PR') return 'pr'
  if (raw === 'Issue') return 'issue'
  return 'neutral'
}

/** Kind pill glyph; undefined for anything but PR/Issue. */
export function kindIcon(item: InboxItem): Component | undefined {
  const style = kindStyle(item)
  if (style === 'pr') return IconGitPullRequest
  if (style === 'issue') return IconCircleDot
  return undefined
}

/** Container/context label: repo, channel, dashboard — whatever the source
 *  scopes its items to. '' when the payload carries none. */
export function container(item: InboxItem): string {
  return canonicalPayload(item).repo
}

/** container() plus a short ordinal badge (" #num") when num is positive. */
export function containerLine(item: InboxItem): string {
  const payload = canonicalPayload(item)
  return payload.repo + (payload.num > 0 ? ` #${payload.num}` : '')
}

/** Actor byline; '' when the payload carries none. */
export function byline(item: InboxItem): string {
  return canonicalPayload(item).author
}

/** Markdown body for the detail pane. */
export function body(item: InboxItem): string {
  return canonicalPayload(item).body
}

/** Reduces a markdown body to its first non-empty line, with heading, list,
 *  link, and emphasis markup stripped — a plain-text one-liner for the feed
 *  row. */
export function bodySnippet(text: string): string {
  for (const raw of text.split('\n')) {
    const line = raw
      .replace(/^#{1,6}\s+/, '')
      .replace(/^\s*[-*+]\s+\[[ xX]\]\s+/, '')
      .replace(/^\s*[-*+>]\s+/, '')
      .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/[*_`~]/g, '')
      .trim()
    if (line) return line
  }
  return ''
}

/** One-line body snippet for the feed row. */
export function snippet(item: InboxItem): string {
  return bodySnippet(body(item))
}

/** Haystack matchesSearch filters the feed against: title, container line,
 *  byline, kind label, source label, and the body snippet. */
export function searchText(item: InboxItem): string {
  return [item.title, containerLine(item), byline(item), kindLabel(item), presentationFor(item.sourceKind).sourceLabel, snippet(item)]
    .join(' ')
}

/** Clipboard text for an inbox item's "Copy contents" action: a compact
 *  header (title, repo #num, link) followed by the raw markdown body —
 *  self-contained enough to paste into a session prompt or a message
 *  without the reader needing the app open. */
export function clipboardText(item: InboxItem): string {
  const payload = canonicalPayload(item)
  const reference = [payload.repo && payload.num ? `${payload.repo} #${payload.num}` : '', item.url].filter(Boolean).join(' · ')
  const header = [item.title, reference].filter(Boolean).join('\n')
  const trimmedBody = payload.body.trim()
  return trimmedBody ? `${header}\n\n${trimmedBody}` : header
}

// ── Provider-variant adapter registry ───────────────────────────────────────

/** Context the host owns that presentation needs beyond the item. */
export interface PresentationContext {
  /** Source node id → configured icon key (useFeedState.sourceIcons). */
  sourceIcons?: Record<string, string>
  /** Source node id → uploaded mark image data URL. Takes precedence over the
   *  glyph when set. */
  sourceImages?: Record<string, string>
}

/** The ONLY provider-variant surface — everything else is a canonical
 *  projection above, shared by every source. */
export interface ItemPresentation {
  /** Human source label: "GitHub", "Webhook"; the default adapter echoes
   *  the raw sourceKind. */
  sourceLabel: string
  /** Resolved glyph component for the source badge; also the fallback when
   *  markImage is set but its image fails to load. */
  mark(item: InboxItem, ctx?: PresentationContext): Component
  /** Uploaded mark image data URL when the source has one; undefined otherwise. */
  markImage?(item: InboxItem, ctx?: PresentationContext): string | undefined
}

const githubPresentation: ItemPresentation = {
  sourceLabel: 'GitHub',
  mark: () => GithubMark,
}

// Gitea's logomark is a fixed two-colour drawing with no currentColor part,
// so GiteaMark wraps the bundled asset and this needs no markImage of its own.
const giteaPresentation: ItemPresentation = {
  sourceLabel: 'Gitea',
  mark: () => GiteaMark,
}

const grafanaPresentation: ItemPresentation = {
  sourceLabel: 'Grafana',
  // The logo is a gradient, so it renders as an image, not a currentColor glyph;
  // IconActivity is only the fallback if the bundled asset fails to load.
  mark: () => IconActivity,
  markImage: () => grafanaLogo,
}

// An inline mark rather than a markImage: PostHog's logomark is part brand
// gradient and part currentColor, and an <img> cannot inherit the theme's
// text color the way the inline SVG does.
const posthogPresentation: ItemPresentation = {
  sourceLabel: 'PostHog',
  mark: () => PostHogMark,
}

const execPresentation: ItemPresentation = {
  sourceLabel: 'Command',
  mark: (item, ctx) => feedIconComponent(ctx?.sourceIcons?.[item.sourceScope] || defaultExecSourceIcon),
  markImage: (item, ctx) => ctx?.sourceImages?.[item.sourceScope],
}

// A feed is a protocol, not a provider: there is no vendor mark to wear, so
// the node's own icon or uploaded image is the whole of it — the same shape
// webhook and exec take.
const rssPresentation: ItemPresentation = {
  sourceLabel: 'Feed',
  mark: (item, ctx) => feedIconComponent(ctx?.sourceIcons?.[item.sourceScope] || defaultRssSourceIcon),
  markImage: (item, ctx) => ctx?.sourceImages?.[item.sourceScope],
}

const webhookPresentation: ItemPresentation = {
  sourceLabel: 'Webhook',
  mark: (item, ctx) => feedIconComponent(ctx?.sourceIcons?.[item.sourceScope] || defaultWebhookSourceIcon),
  markImage: (item, ctx) => ctx?.sourceImages?.[item.sourceScope],
}

/** Registry lookup; an unknown/absent sourceKind gets the default adapter —
 *  a neutral mark and its raw sourceKind as the label — instead of
 *  guessing at a provider. Renders today only for source kinds that don't
 *  exist yet (e.g. `generic` test observations in Trash). */
export function presentationFor(sourceKind: string | undefined): ItemPresentation {
  if (sourceKind === 'github') return githubPresentation
  if (sourceKind === 'gitea') return giteaPresentation
  if (sourceKind === 'grafana') return grafanaPresentation
  if (sourceKind === 'posthog') return posthogPresentation
  if (sourceKind === 'webhook') return webhookPresentation
  if (sourceKind === 'exec') return execPresentation
  if (sourceKind === 'rss') return rssPresentation
  return { sourceLabel: sourceKind ?? '', mark: () => IconInbox }
}

// ── Single source-kind list ─────────────────────────────────────────────────
// Each backend source node's config module exports its sourceKind next to
// its node `type` — the only place a flow node type maps to a sourceKind, so
// provider N+1 adds a node module and never touches a hand-maintained list
// here.

const SOURCE_KIND_BY_NODE_TYPE: Record<string, string> = {
  [githubSourceNode.type]: githubSourceNode.sourceKind,
  [giteaSourceNode.type]: giteaSourceNode.sourceKind,
  [grafanaMetricsSourceNode.type]: grafanaMetricsSourceNode.sourceKind,
  [grafanaAlertsSourceNode.type]: grafanaAlertsSourceNode.sourceKind,
  [grafanaIRMAlertsSourceNode.type]: grafanaIRMAlertsSourceNode.sourceKind,
  [posthogErrorsSourceNode.type]: posthogErrorsSourceNode.sourceKind,
  [posthogAlertsSourceNode.type]: posthogAlertsSourceNode.sourceKind,
  [webhookSourceNode.type]: webhookSourceNode.sourceKind,
  [execSourceNode.type]: execSourceNode.sourceKind,
  [rssSourceNode.type]: rssSourceNode.sourceKind,
}

/** Flow node type → inbox item sourceKind; null for non-source node types. */
export function sourceKindForNodeType(nodeType: string): string | null {
  return SOURCE_KIND_BY_NODE_TYPE[nodeType] ?? null
}

// ── Sidebar summary ─────────────────────────────────────────────────────────

/** Sidebar source-count summary for a profile's source nodes: the total number
 *  of sources across every kind. Feeds mix providers, so the summary stays
 *  provider-neutral — "<N> source(s)", or "No sources" when there are none. */
export function sourceSummary(countByKind: ReadonlyMap<string, number>): string {
  const total = [...countByKind.values()].reduce((sum, count) => sum + Math.max(count, 0), 0)
  if (total === 0) return 'No sources'
  return `${total} source${total === 1 ? '' : 's'}`
}
