// launch-chat is a terminal node (1 in / 0 out): each arriving item opens a
// chat in an agent workspace, with the rendered prompt as its first message.
// It is the inline form of an action node pointing at a workspace
// launch-session action, for a flow that is the only caller.

import IconMessageSquarePlus from '~icons/lucide/message-square-plus'

export const type = 'launch-chat'
export const role = 'output' as const

/** Mirrors Go's flow.LaunchChatConfig. */
export interface Config {
  /** Agent workspace directory name; required. */
  workspace: string
  /** Go template rendering the opening message; required. */
  prompt: string
}

export const unread = false

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Launch chat'
export const category = 'Destinations' as const
export const glyph = IconMessageSquarePlus
// Orange, shared with the Action node: see launch-session/config.ts.
export const accentToken = 'var(--color-node-orange)'
export const tint = 'var(--color-node-orange-tint)'
export const outputs = 0

export const defaults: Config = {
  workspace: '',
  prompt: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!config.workspace?.trim()) errors.push('workspace is required')
  if (!config.prompt?.trim()) errors.push('prompt is required')
  return errors
}
