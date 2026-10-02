// launch-session is a terminal node: an action node's launch-session declared
// inline.

import IconSquareTerminal from '~icons/lucide/square-terminal'

export const type = 'launch-session'
export const role = 'output' as const

/** Mirrors Go's flow.LaunchSessionConfig. */
export interface Config {
  repo: string
  prompt: string
  agent?: string
  sessionName?: string
}

export const unread = false

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Launch session'
export const category = 'Destinations' as const
export const glyph = IconSquareTerminal
// Shares the Action node's color: a launch node is an inline action.
export const accentToken = 'var(--color-node-orange)'
export const tint = 'var(--color-node-orange-tint)'
export const outputs = 0

export const ITEM_REMOTE = '{{ .ItemRemote }}'

export type RepoMode = 'item' | 'configured' | 'template'

export function repoMode(repo: string): RepoMode {
  if (repo.trim() === ITEM_REMOTE) return 'item'
  return repo.includes('{{') ? 'template' : 'configured'
}

export const defaults: Config = {
  repo: ITEM_REMOTE,
  prompt: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!config.repo?.trim()) errors.push('repo is required')
  if (!config.prompt?.trim()) errors.push('prompt is required')
  return errors
}
