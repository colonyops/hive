// Runs on the backend (internal/app/sources/exec); role 'source' means no runtime.ts here.

import IconTerminal from '~icons/lucide/terminal'
import { DURATION, intervalError } from '../../lib/sourceInterval'

export const type = 'sources.exec'
export const role = 'source' as const
export const sourceKind = 'exec'

export interface Config {
  /** The shell command line, run through `sh -c` on each poll tick. */
  command: string
  /** Go duration string bounding one run. Required. */
  timeout: string
  cwd?: string
  env?: Record<string, string>
  /** Go duration string: the shortest time between runs. */
  interval?: string
  icon?: string
  /** Content hash of an uploaded image shown as the feed mark instead of `icon`. */
  image?: string
}

export const label = 'Command source'
export const category = 'Sources' as const
// A shell command rather than a product, so there is no vendor mark to wear
// and it keeps the generic source hue the branded sources have moved off.
export const glyph = IconTerminal
export const accentToken = 'var(--color-node-blue)'
export const tint = 'var(--color-node-blue-tint)'

export const defaults: Config = {
  command: '',
  timeout: '30s',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!(config.command ?? '').trim()) errors.push('a command is required')

  const timeout = (config.timeout ?? '').trim()
  if (!timeout) errors.push('a timeout is required, like "30s"')
  else if (!DURATION.test(timeout)) errors.push('timeout must be a duration like "30s", not a bare number')

  const interval = intervalError(config.interval)
  if (interval) errors.push(interval)

  const cwd = (config.cwd ?? '').trim()
  if (cwd && !cwd.startsWith('/') && cwd !== '~' && !cwd.startsWith('~/')) {
    errors.push('cwd must be an absolute path')
  }

  if (config.image && !/^[0-9a-f]{32}$/.test(config.image)) errors.push('image is not a valid mark reference')
  return errors
}
