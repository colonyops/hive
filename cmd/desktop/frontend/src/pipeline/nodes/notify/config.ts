// notify is a terminal node (1 in / 0 out): the arriving msg enqueues an
// output_command that the backend delivers as a native OS notification, with
// `title`/`body` rendered as Go text/templates over the message. See
// nodes/action/config.ts for the parallel terminal — `sink` here is the
// engine's single source of truth for commit-tagging this node.

import IconBell from '~icons/lucide/bell'

export const type = 'notify'
export const role = 'output' as const

/** Mirrors Go's flow.NotifyConfig. */
export interface Config {
  /** Go text/template rendered over the message; required. */
  title: string
  /** Go text/template rendered over the message; optional. */
  body?: string
  /** Native interruption level. Absent means `defaultSeverity`. */
  severity?: string
  /** Absent means "play the notification sound"; only `false` silences it. */
  sound?: boolean
  /**
   * Per-item delivery floor in seconds: once this node interrupts about an
   * item, it stays quiet about that same item for this long. Absent means
   * `defaultCooldownSeconds`; an explicit 0 disables the cooldown.
   */
  cooldownSeconds?: number
}

/** The severities a notify node may declare — mirrors Go's notifySeverities. */
export const severities = ['info', 'success', 'warning', 'error'] as const

/** The severity used when the node declares none. */
export const defaultSeverity = 'info'

/** Longest templates the editor accepts — mirror Go's notify*MaxLen. */
export const titleMaxLen = 200
export const bodyMaxLen = 1000

/** The cooldown used when the node declares none — mirrors Go's NotifyCooldownDefault. */
export const defaultCooldownSeconds = 300

/** Notifications are transient interrupts, not feed items. */
export const unread = false

/**
 * The notify node's durable key is the flow-qualified node id, so two notify
 * nodes fed by the same message deduplicate (and cool down) independently.
 */

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Notify'
export const category = 'Destinations' as const
export const glyph = IconBell
// Purple — the two existing destinations already own green (Feed) and orange
// (Action), so a notify node needs a third hue to stay distinguishable at a
// glance on the canvas.
export const accentToken = 'var(--color-node-purple)'
export const tint = 'var(--color-node-purple-tint)'
/** Terminal node — 0 outputs. */
export const outputs = 0

export const defaults: Config = {
  title: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!config.title || !config.title.trim()) errors.push('title is required')
  if ((config.title?.length ?? 0) > titleMaxLen) errors.push(`Title must be at most ${titleMaxLen} characters.`)
  if ((config.body?.length ?? 0) > bodyMaxLen) errors.push(`Body must be at most ${bodyMaxLen} characters.`)
  if (config.severity && !severities.includes(config.severity as (typeof severities)[number])) {
    errors.push(`Severity "${config.severity}" is not supported.`)
  }
  if (config.cooldownSeconds !== undefined && (config.cooldownSeconds < 0 || !Number.isInteger(config.cooldownSeconds))) {
    errors.push('Cooldown must be a whole, non-negative number of seconds.')
  }
  return errors
}
