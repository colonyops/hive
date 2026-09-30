// sources.webhook is a source node (0 in / 1 out): the push-driven counterpart
// to sources.github. The node declares a path on the desktop's local webhook
// listener; JSON POSTed to http://127.0.0.1:<port>/hooks/<path> is ingested by
// Go and appended to the event log under topic "source:<flowId>/<nodeId>" —
// the frontend never executes the source, it only consumes the msgs the
// backend already appended (role: 'source', no runtime.ts), exactly like
// sources.github. See internal/app/sources/webhook/webhook_source.go.

import IconWebhook from '~icons/lucide/webhook'

import { isFeedIcon } from '../../../lib/feedIcons'

export const type = 'sources.webhook'
export const role = 'source' as const
// The inbox item sourceKind this node's items carry — see sources.github's
// config.ts for why this lives next to `type`.
export const sourceKind = 'webhook'

export interface Config {
  /**
   * Endpoint path under /hooks/: one or more slug segments separated by "/"
   * (lowercase letters, digits, "-", "_"), e.g. "ci-alerts" or "ci/deploys".
   */
  path: string
  /**
   * Optional shared secret; when set, senders must present the same value in
   * the X-Hive-Secret request header.
   */
  secret?: string
  /**
   * Optional glyph feed rows render for this node's items, from the shared
   * feed icon set. Empty means the default webhook glyph.
   */
  icon?: string
  /** Content hash of an uploaded image shown as the feed mark instead of `icon`. */
  image?: string
}

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Webhook source'
export const category = 'Sources' as const
// A protocol rather than a product, so there is no vendor mark to wear and
// it keeps the generic source hue the branded sources have moved off.
export const glyph = IconWebhook
export const accentToken = 'var(--color-node-blue)'
export const tint = 'var(--color-node-blue-tint)'

export const defaults: Config = {
  path: '',
}

// A fresh node gets a generated path so two webhook sources never silently
// share an endpoint (every node declaring a path receives its deliveries) and
// so an endpoint is not guessable from the flow name. The secret stays opt-in
// — generated on demand from the editor, not imposed on every node.
export function freshConfig(): Partial<Config> {
  return { path: randomPath() }
}

// Both generators draw from crypto.getRandomValues: the secret is a real
// credential, and sharing one primitive keeps the path unguessable too.
// Alphabets are chosen so output always satisfies validate() below.
const PATH_ALPHABET = 'abcdefghijklmnopqrstuvwxyz0123456789'
const SECRET_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'

function randomChars(alphabet: string, length: number): string {
  const bytes = new Uint8Array(length)
  crypto.getRandomValues(bytes)
  // The alphabets divide 256 evenly (36 does not, but the modulo bias over a
  // 36-symbol alphabet is negligible for an endpoint slug; the 64-symbol
  // secret alphabet is unbiased).
  return Array.from(bytes, (byte) => alphabet[byte % alphabet.length]).join('')
}

/** A slug-shaped endpoint path, e.g. "hook-k3m9x2qp". */
export function randomPath(): string {
  return `hook-${randomChars(PATH_ALPHABET, 8)}`
}

/** A 32-character shared secret for the X-Hive-Secret header. */
export function randomSecret(): string {
  return randomChars(SECRET_ALPHABET, 32)
}

const SEGMENT = /^[a-z0-9][a-z0-9_-]*$/

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  const path = config.path ?? ''
  if (!path.trim()) {
    errors.push('a webhook source requires a path')
  } else if (path.length > 128) {
    errors.push('path caps at 128 characters')
  } else if (!path.split('/').every((segment) => SEGMENT.test(segment))) {
    errors.push('path must be lowercase slug segments, e.g. "ci-alerts" or "ci/deploys"')
  }
  const secret = config.secret ?? ''
  if (secret.length > 128) errors.push('secret caps at 128 characters')
  else if (secret && !/^[!-~]+$/.test(secret)) errors.push('secret must be printable ASCII without spaces')
  if (config.icon && !isFeedIcon(config.icon)) errors.push(`"${config.icon}" is not a supported feed icon`)
  if (config.image && !/^[0-9a-f]{32}$/.test(config.image)) errors.push('image is not a valid mark reference')
  return errors
}
