// Runs on the backend (internal/app/sources/grafana); role 'source' means no runtime.ts here.

import GrafanaMark from '../../../components/marks/GrafanaMark.vue'
import { intervalError } from '../../lib/sourceInterval'

export const type = 'sources.grafana_irm_alerts'
export const role = 'source' as const
export const sourceKind = 'grafana'

export interface Config {
  /**
   * A ref and never a token: flows/ is dotfiles-managed, so an embedded token
   * would be a token in a git repo.
   */
  credential: string
  /** An IRM integration id, the unit a squad's upstream routes deliver to. */
  integration: string
  /** An IRM team id. */
  team: string
  /** Go duration string: the shortest time between fetches. */
  interval?: string
}

export const label = 'Grafana IRM alerts source'
export const category = 'Sources' as const
// Grafana's own mark, shared with the Integrations screen and the inbox
// source badge. Every Grafana source wears it: the node title is what
// separates alerts from IRM from metrics.
export const glyph = GrafanaMark
/** A product logomark, not a lucide glyph — see NodeTypeDefinition.logoMark. */
export const logoMark = true
export const accentToken = 'var(--color-brand-grafana)'
export const tint = 'var(--color-brand-grafana-tint)'

export const defaults: Config = {
  credential: '',
  integration: '',
  team: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  const credential = (config.credential ?? '').trim()
  if (!credential) {
    errors.push('a source needs a connected Grafana stack')
  } else if (!/^grafana\/[^/]+$/.test(credential)) {
    errors.push('credential must look like "grafana/<account>"')
  }
  const interval = intervalError(config.interval)
  if (interval) errors.push(interval)
  return errors
}
