// Runs on the backend (internal/app/sources/posthog); role 'source' means no runtime.ts here.

import PostHogMark from '../../../components/marks/PostHogMark.vue'
import { intervalError } from '../../lib/sourceInterval'

export const type = 'sources.posthog_alerts'
export const role = 'source' as const
export const sourceKind = 'posthog'

export interface Config {
  /**
   * A ref and never a token: flows/ is dotfiles-managed, so an embedded token
   * would be a token in a git repo.
   */
  credential: string
  firing_only?: boolean
  /** Go duration string: the shortest time between fetches. */
  interval?: string
}

export const label = 'PostHog insight alerts source'
export const category = 'Sources' as const
// PostHog's own mark, shared with the Integrations screen and the inbox
// source badge. Both PostHog sources wear it; the node title separates them.
export const glyph = PostHogMark
/** A product logomark, not a lucide glyph — see NodeTypeDefinition.logoMark. */
export const logoMark = true
export const accentToken = 'var(--color-brand-posthog)'
export const tint = 'var(--color-brand-posthog-tint)'

export const defaults: Config = {
  credential: '',
  firing_only: false,
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  const credential = (config.credential ?? '').trim()
  if (!credential) {
    errors.push('a source needs a connected PostHog project')
  } else if (!/^posthog\/[^/]+$/.test(credential)) {
    errors.push('credential must look like "posthog/<account>"')
  }
  const interval = intervalError(config.interval)
  if (interval) errors.push(interval)
  return errors
}
