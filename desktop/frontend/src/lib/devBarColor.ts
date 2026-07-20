// Deterministic branch → color mapping for the dev bar. Concurrently-running
// dev instances (one per branch/worktree) each get a stable, distinct accent so
// they're tellable apart at a glance — the same branch always lands on the same
// color across restarts and windows.

// A spread of bright hues (Tailwind ~400 values). All are light enough that
// readableTextColor picks dark text for most, keeping the bar legible.
export const DEV_BAR_COLORS = [
  '#f59e0b', // amber
  '#fb923c', // orange
  '#f87171', // red
  '#fb7185', // rose
  '#f472b6', // pink
  '#e879f9', // fuchsia
  '#a78bfa', // violet
  '#818cf8', // indigo
  '#60a5fa', // blue
  '#38bdf8', // sky
  '#22d3ee', // cyan
  '#2dd4bf', // teal
  '#34d399', // emerald
  '#4ade80', // green
  '#a3e635', // lime
] as const

// FNV-1a (32-bit): a small, well-distributed string hash. Math.imul keeps the
// multiply in 32-bit space; the final `>>> 0` yields an unsigned int.
export function hashBranch(branch: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < branch.length; i++) {
    h ^= branch.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

/** The palette color assigned to a branch — stable for a given name. */
export function colorForBranch(branch: string): string {
  return DEV_BAR_COLORS[hashBranch(branch) % DEV_BAR_COLORS.length]
}

/**
 * Black or white, whichever reads better on `hex` (a `#rrggbb` string).
 * Uses the YIQ brightness heuristic; falls back to dark text on a bad input.
 */
export function readableTextColor(hex: string): '#0b0b0c' | '#ffffff' {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex)
  if (!m) return '#0b0b0c'
  const n = parseInt(m[1], 16)
  const r = (n >> 16) & 0xff
  const g = (n >> 8) & 0xff
  const b = n & 0xff
  const yiq = (r * 299 + g * 587 + b * 114) / 1000
  return yiq >= 140 ? '#0b0b0c' : '#ffffff'
}
