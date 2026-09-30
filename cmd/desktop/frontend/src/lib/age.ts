const relativeFormat = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

const relativeUnits: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ['year', 31_536_000],
  ['month', 2_592_000],
  ['week', 604_800],
  ['day', 86_400],
  ['hour', 3600],
  ['minute', 60],
]

// Spelled-out relative time ("3 days ago") for settings surfaces, where the
// terse feed form below would read as a riddle. Timestamps in the future are
// phrased as such rather than clamped — a clock skew should look wrong.
export function relativeTimeLabel(timestamp: number, now = Date.now()): string {
  const seconds = Math.round((timestamp - now) / 1000)
  for (const [unit, size] of relativeUnits) {
    if (Math.abs(seconds) >= size) return relativeFormat.format(Math.round(seconds / size), unit)
  }
  return 'just now'
}

// Deterministic relative timestamps derived from inbox event time.
export function relativeAge(timestamp: number, now = Date.now()): string {
  const seconds = Math.max(0, Math.floor((now - timestamp) / 1000))
  if (seconds < 60) return 'now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d`
  return `${Math.floor(days / 7)}w`
}
