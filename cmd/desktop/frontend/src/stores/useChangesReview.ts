import { useStorage } from '@vueuse/core'
import { errorText } from '../lib/appError'
import { hiveUiPlatform } from '../lib/hiveUiPlatform'
import type { DiffSpan } from '../lib/diffReview'
import {
  ChangesFeedback,
  ChangesTargets,
  SendChanges,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice'
import type { AgentTarget } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import type { DiffBase } from '../composables/useSessionChanges'
import { defineStore } from './defineStore'

export interface ChangesComment extends DiffSpan {
  id: string
  path: string
  /** The comparison the comment was made in: removed lines are only numbered within it. */
  base: DiffBase
  /** The ref a branch comparison used. Missing on a comment from before there was a choice. */
  against?: string
  /** The commit a commit comparison showed. */
  commit?: string
  /** Its lines are numbered as in the index, which the agent reads from this flag. */
  staged?: boolean
  /** The file's version when the comment was made, to tell when the lines have moved under it. */
  version: string
  text: string
}

/** A review that went to the agent, kept in case the agent never acted on it. */
export interface SentReview {
  id: string
  sentAt: number
  comments: ChangesComment[]
}

/** tmux accepting a paste says nothing of the agent reading it, so a few sends stay recoverable. */
const MAX_SENT_REVIEWS = 3

/**
 * Comments on a session's diffs, waiting to go to its agent. In browser storage
 * rather than hive.db: they anchor to lines an agent is about to move, are gone
 * the moment they are sent, and belong to this machine's sitting. A relaunch
 * still must not lose what the user wrote.
 */
export const useChangesReview = defineStore('changesReview', () => {
  const sessions = useStorage<Record<string, ChangesComment[]>>('hive.changes.comments', {})
  const sent = useStorage<Record<string, SentReview[]>>('hive.changes.sent', {})

  function comments(sessionId: string): readonly ChangesComment[] {
    return sessions.value[sessionId] ?? []
  }

  function write(sessionId: string, next: ChangesComment[]): void {
    const rest = { ...sessions.value }
    if (next.length) rest[sessionId] = next
    else delete rest[sessionId]
    sessions.value = rest
  }

  function add(sessionId: string, comment: Omit<ChangesComment, 'id'>): ChangesComment {
    const added = { ...comment, id: crypto.randomUUID() }
    write(
      sessionId,
      [...comments(sessionId), added].sort((a, b) => a.path.localeCompare(b.path) || a.startLine - b.startLine),
    )
    return added
  }

  function update(sessionId: string, id: string, text: string): void {
    write(
      sessionId,
      comments(sessionId).map((comment) => (comment.id === id ? { ...comment, text } : comment)),
    )
  }

  function remove(sessionId: string, id: string): void {
    write(
      sessionId,
      comments(sessionId).filter((comment) => comment.id !== id),
    )
  }

  async function targets(sessionId: string): Promise<AgentTarget[]> {
    return (await ChangesTargets(sessionId)) ?? []
  }

  /** Delivers the review and clears it. A failed delivery keeps every comment. Returns why it failed, or null. */
  async function send(sessionId: string, pane: string): Promise<string | null> {
    try {
      await SendChanges(sessionId, pane, [...comments(sessionId)])
    } catch (err) {
      return errorText(err, 'Could not send the feedback.')
    }
    const held = [...comments(sessionId)]
    write(sessionId, [])
    sent.value = {
      ...sent.value,
      [sessionId]: [{ id: crypto.randomUUID(), sentAt: Date.now(), comments: held }, ...sentReviews(sessionId)].slice(
        0,
        MAX_SENT_REVIEWS,
      ),
    }
    return null
  }

  function sentReviews(sessionId: string): readonly SentReview[] {
    return sent.value[sessionId] ?? []
  }

  /** Makes a sent review's comments unsent again, after any the user has written since. */
  function restore(sessionId: string, reviewId: string): void {
    const review = sentReviews(sessionId).find((candidate) => candidate.id === reviewId)
    if (!review) return
    write(
      sessionId,
      [...comments(sessionId), ...review.comments].sort(
        (a, b) => a.path.localeCompare(b.path) || a.startLine - b.startLine,
      ),
    )
    sent.value = { ...sent.value, [sessionId]: sentReviews(sessionId).filter((other) => other.id !== reviewId) }
  }

  /** Puts the feedback text on the clipboard. Returns why it could not, or null. */
  async function copy(held: readonly ChangesComment[]): Promise<string | null> {
    try {
      await hiveUiPlatform.clipboard.writeText((await ChangesFeedback([...held])) ?? '')
      return null
    } catch (err) {
      return errorText(err, 'Could not copy the comments.')
    }
  }

  /** Drops everything held for a session that no longer exists. */
  function forget(sessionId: string): void {
    write(sessionId, [])
    const rest = { ...sent.value }
    delete rest[sessionId]
    sent.value = rest
  }

  return { comments, add, update, remove, targets, send, sentReviews, restore, copy, forget }
})
