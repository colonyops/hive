// create-task is a terminal node: it files one hive task per item.

import IconListPlus from '~icons/lucide/list-plus'

export const type = 'create-task'
export const role = 'output' as const

/** Mirrors Go's flow.CreateTaskConfig. */
export interface Config {
  title: string
  description?: string
  repo?: string
  parent?: string
  tags?: string[]
}

export const unread = false

/** Mirrors Go's tagset.MaxLength. */
const MAX_TAG_LENGTH = 40

/** Splits the comma-separated tags field. A comma cannot be part of a tag. */
export function parseTags(text: string): string[] | undefined {
  const tags = text
    .split(',')
    .map((tag) => tag.trim())
    .filter((tag) => tag.length > 0)
  return tags.length > 0 ? tags : undefined
}

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Create task'
export const category = 'Destinations' as const
export const glyph = IconListPlus
// Shares the Action node's color: this node is an inline action.
export const accentToken = 'var(--color-node-orange)'
export const tint = 'var(--color-node-orange-tint)'
export const outputs = 0

export const defaults: Config = {
  title: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!config.title?.trim()) errors.push('title is required')
  const parent = config.parent ?? ''
  if (/\s/.test(parent) || parent.includes('{{')) errors.push('parent must be a task id')
  const tags = config.tags ?? []
  if (tags.some((tag) => tag.includes('{{'))) errors.push('tags must be literal tags, not templates')
  else if (tags.some((tag) => [...tag].length > MAX_TAG_LENGTH))
    errors.push(`tags must be at most ${MAX_TAG_LENGTH} characters`)
  return errors
}
