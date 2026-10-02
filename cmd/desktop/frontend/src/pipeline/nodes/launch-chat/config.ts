// launch-chat is a terminal node: an action node's workspace launch-session
// declared inline.

import IconMessageSquarePlus from '~icons/lucide/message-square-plus'

export const type = 'launch-chat'
export const role = 'output' as const

/** Mirrors Go's flow.LaunchChatConfig. */
export interface Config {
  workspace: string
  prompt: string
}

export const unread = false

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Launch chat'
export const category = 'Destinations' as const
export const glyph = IconMessageSquarePlus
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
