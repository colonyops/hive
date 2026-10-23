export const tag = {
  // surface
  feed: '@feed',
  palette: '@palette',
  drive: '@drive',
  flows: '@flows',
  actions: '@actions',
  terminal: '@terminal',
  chats: '@chats',
  integrations: '@integrations',
  notifications: '@notifications',
  onboarding: '@onboarding',
  settings: '@settings',
  diagnostics: '@diagnostics',
  // priority
  critical: '@critical',
} as const

export type Tag = (typeof tag)[keyof typeof tag]

export type Kind = 'smoke' | 'workflows' | 'features'

export const surfaceTags: Tag[] = [
  tag.feed,
  tag.palette,
  tag.drive,
  tag.flows,
  tag.actions,
  tag.terminal,
  tag.chats,
  tag.integrations,
  tag.notifications,
  tag.onboarding,
  tag.settings,
  tag.diagnostics,
]

const allTags: string[] = Object.values(tag)

/** Returns one message per rule the tags break; an empty list means they pass. */
export function tagProblems(tags: readonly string[], kind: Kind): string[] {
  const problems: string[] = []

  const unknown = tags.filter((t) => !allTags.includes(t))
  if (unknown.length > 0) {
    problems.push(`unknown tag(s) ${unknown.join(', ')}; add them to fixtures/tags.ts`)
  }

  if (!tags.some((t) => (surfaceTags as string[]).includes(t))) {
    problems.push(`no surface tag; add at least one of ${surfaceTags.join(', ')}`)
  }

  // --grep @critical is the smoke and workflow subset; a feature test there
  // would widen it.
  if (tags.includes(tag.critical) && kind === 'features') {
    problems.push(`${tag.critical} is only for smoke and workflow tests`)
  }

  return problems
}
