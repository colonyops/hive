// The cadence floor every pull source node offers, shared so eight node
// editors describe and check one field the same way. Go's
// connector.ValidateInterval is authoritative; this is the editor's copy.

/** Mirrors Go's connector.Duration: a Go duration string, never a bare number. */
export const DURATION = /^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)([\d.]+(ns|us|µs|ms|s|m|h))*$/

export const INTERVAL_LABEL = 'Minimum interval'
export const INTERVAL_PLACEHOLDER = 'every poll'
export const INTERVAL_HINT = 'Shortest time between fetches, for a source not worth polling every tick. Rounds up to the next tick; a manual refresh ignores it.'

/** Returns the error for an unusable interval, or null when it is fine. */
export function intervalError(interval: string | undefined): string | null {
  const value = (interval ?? '').trim()
  if (!value) return null
  return DURATION.test(value) ? null : 'interval must be a duration like "1h", not a bare number'
}
