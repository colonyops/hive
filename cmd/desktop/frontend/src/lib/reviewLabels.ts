import type { Component } from 'vue'
import IconAlertTriangle from '~icons/lucide/triangle-alert'
import IconCheck from '~icons/lucide/check'
import IconCircleHelp from '~icons/lucide/circle-help'
import IconFlaskConical from '~icons/lucide/flask-conical'
import IconLightbulb from '~icons/lucide/lightbulb'
import IconScissors from '~icons/lucide/scissors'
import IconSearchCheck from '~icons/lucide/search-check'
import IconTag from '~icons/lucide/tag'
import IconX from '~icons/lucide/x'

/**
 * A preset comment a reviewer applies in one keystroke. A label is stored as
 * an ordinary comment whose text starts with its title, so the agent reads it
 * like any other comment and the card can show its icon.
 */
export type ReviewLabel = { key: string; title: string; icon: Component }

/** How many labels the composer has keys for. */
export const maxReviewLabels = 4

/** The icons a label in settings.yaml can name, by lucide name. */
export const reviewLabelIcons: Readonly<Record<string, Component>> = {
  'circle-help': IconCircleHelp,
  'search-check': IconSearchCheck,
  'flask-conical': IconFlaskConical,
  check: IconCheck,
  tag: IconTag,
  'triangle-alert': IconAlertTriangle,
  lightbulb: IconLightbulb,
  scissors: IconScissors,
  x: IconX,
}

/**
 * The labels settings.yaml configures, keyed 1 to 4 by position. An unknown
 * icon draws as a tag, and anything past the fourth is dropped: Go validates
 * both, but a label must never take a key the composer does not have.
 */
export function resolveReviewLabels(labels: readonly ReviewLabelSetting[]): ReviewLabel[] {
  return labels.slice(0, maxReviewLabels).map((label, index) => ({
    key: String(index + 1),
    title: label.title,
    icon: reviewLabelIcons[label.icon] ?? IconTag,
  }))
}

/** A label as settings.yaml stores it. */
export type ReviewLabelSetting = { title: string; icon: string }

/** The shipped labels, which the composer offers until settings load. */
export const defaultReviewLabelSettings: readonly ReviewLabelSetting[] = [
  { title: 'Clarify this', icon: 'circle-help' },
  { title: 'Verify this', icon: 'search-check' },
  { title: 'Needs tests', icon: 'flask-conical' },
  { title: 'Looks good', icon: 'check' },
]

export const defaultReviewLabels = resolveReviewLabels(defaultReviewLabelSettings)

/** The comment text a label saves, with the reviewer's own words after it. */
export function labelText(label: ReviewLabel, text: string): string {
  const extra = text.trim()
  return extra ? `${label.title}: ${extra}` : label.title
}

/** The label a comment's text starts with, if any. */
export function labelOf(labels: readonly ReviewLabel[], text: string): ReviewLabel | null {
  return labels.find((label) => text === label.title || text.startsWith(`${label.title}: `)) ?? null
}
