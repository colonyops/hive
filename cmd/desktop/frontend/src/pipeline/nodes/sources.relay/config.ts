// Runs on the backend (cmd/desktop/internal/app/sources/relay); role 'source' means no runtime.ts here.

import IconRadioTower from '~icons/lucide/radio-tower'
import { DURATION, intervalError } from '../../lib/sourceInterval'

export const type = 'sources.relay'
export const role = 'source' as const
export const sourceKind = 'relay'

export type Dialect = 'auto' | 'generic' | 'ntfy' | 'gotify' | 'healthchecks'

export interface DialectOption {
  value: Dialect
  label: string
}

/** Mirrors validateDialect in cmd/desktop/internal/app/sources/relay/config.go. */
export const DIALECTS: DialectOption[] = [
  { value: 'auto', label: 'Auto-detect' },
  { value: 'generic', label: 'Generic' },
  { value: 'ntfy', label: 'ntfy' },
  { value: 'gotify', label: 'Gotify' },
  { value: 'healthchecks', label: 'Healthchecks' },
]

/** RFC 6901 JSON Pointers over the parsed item; empty means unset. */
export interface RelayMap {
  key?: string
  title?: string
  url?: string
  kind?: string
  state?: string
  /** A resolved state value mapped onto the canonical state it means. */
  states?: Record<string, string>
}

export interface RelayCheck {
  period: string
  grace: string
}

export interface Config {
  /** The connected relay account, written `relay/<host>-<email>`. */
  credential: string
  /** The relay source id this node reads. */
  source: string
  dialect?: Dialect
  map?: RelayMap
  checks?: Record<string, RelayCheck>
  /** Go duration string: the shortest time between fetches. */
  interval?: string
  icon?: string
  /** Content hash of an uploaded image shown as the feed mark instead of `icon`. */
  image?: string
}

export const label = 'Hive Relay'
export const category = 'Sources' as const
export const glyph = IconRadioTower
export const accentToken = 'var(--color-node-blue)'
export const tint = 'var(--color-node-blue-tint)'

export const defaults: Config = {
  credential: '',
  source: '',
  dialect: 'auto',
  interval: '1m',
}

export const MAP_POINTERS = ['key', 'title', 'url', 'kind', 'state'] as const

/** UX-only: Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []

  const credential = (config.credential ?? '').trim()
  if (!credential) errors.push('a source needs a connected Hive Relay account')
  else if (!/^relay\/[^/]+$/.test(credential)) errors.push('credential must look like "relay/<host>-<email>"')

  if (!(config.source ?? '').trim()) errors.push('pick or create a relay source')

  if (config.dialect) {
    const dialect = DIALECTS.find((d) => d.value === config.dialect)
    if (!dialect) errors.push(`format "${config.dialect}" is not one of ${DIALECTS.map((d) => d.value).join(', ')}`)
  }

  for (const [slug, check] of Object.entries(config.checks ?? {})) {
    if (!slug.trim()) errors.push('a check needs a slug')
    const period = (check.period ?? '').trim()
    if (!period) errors.push(`check "${slug}" needs a period like "1h"`)
    else if (!DURATION.test(period)) errors.push(`check "${slug}" period must be a duration like "1h"`)
    const grace = (check.grace ?? '').trim()
    if (grace && !DURATION.test(grace)) errors.push(`check "${slug}" grace must be a duration like "5m"`)
  }

  for (const name of MAP_POINTERS) {
    const pointer = config.map?.[name]
    if (pointer && !pointer.startsWith('/')) errors.push(`map.${name} must be a JSON Pointer starting with "/"`)
  }

  const interval = intervalError(config.interval)
  if (interval) errors.push(interval)

  if (config.image && !/^[0-9a-f]{32}$/.test(config.image)) errors.push('image is not a valid mark reference')
  return errors
}
