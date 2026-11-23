// publish-message is a terminal node: an action node's publish-message
// declared inline.

import IconRadio from '~icons/lucide/radio'

export const type = 'publish-message'
export const role = 'output' as const

/** Mirrors Go's flow.PublishMessageConfig. */
export interface Config {
  topic: string
  message: string
}

export const unread = false

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Publish message'
export const category = 'Destinations' as const
export const glyph = IconRadio
// Shares the Action node's color: this node is an inline action.
export const accentToken = 'var(--color-node-orange)'
export const tint = 'var(--color-node-orange-tint)'
export const outputs = 0

export const defaults: Config = {
  topic: '',
  message: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  const topic = config.topic?.trim() ?? ''
  if (!topic) errors.push('topic is required')
  else if (/[*\s]/.test(topic) || topic.includes('{{')) {
    errors.push('topic must be a literal without spaces, wildcards, or templates')
  }
  if (!config.message?.trim()) errors.push('message is required')
  return errors
}
