// launch-session is a terminal node (1 in / 0 out): each arriving item starts
// a repository-backed hive session. It is the inline form of an action node
// pointing at a launch-session action, for a flow that is the only caller.

import IconSquareTerminal from '~icons/lucide/square-terminal'

export const type = 'launch-session'
export const role = 'output' as const

/** Mirrors Go's flow.LaunchSessionConfig. */
export interface Config {
  /** Go template rendering the repository's remote; required. */
  repo: string
  /** Go template rendering the initial prompt; required. */
  prompt: string
  /** Hive agent profile. Absent means the default agent. */
  agent?: string
  /** Go template rendering the session name. Absent derives one from the node and the item. */
  sessionName?: string
}

export const unread = false

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Launch session'
export const category = 'Destinations' as const
export const glyph = IconSquareTerminal
// Orange, shared with the Action node: a launch node is an action declared
// inline, so it reads as one on the canvas.
export const accentToken = 'var(--color-node-orange)'
export const tint = 'var(--color-node-orange-tint)'
export const outputs = 0

/** The repo template that clones the repository the item belongs to. */
export const ITEM_REMOTE = '{{ .ItemRemote }}'

/**
 * How the editor presents a stored repo. The flow file keeps one string, so
 * the mode is read back from it: anything templated other than ITEM_REMOTE is
 * a hand-written template, and a plain value is a fixed remote.
 */
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
