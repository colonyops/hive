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

/** What a comment is about when it is not a run of rows: one file, or the changes as a whole. */
export type ChangesCommentKind = 'file' | 'review'

export interface ChangesComment extends DiffSpan {
  id: string
  /** Unset for a comment on rows. A file or review comment has no rows: its lines are 0. */
  kind?: ChangesCommentKind
  /** Empty for a review comment. */
  path: string
  /** The comparison the comment was made in: removed lines are only numbered within it. */
  base: DiffBase
  /** The ref a branch comparison used. */
  against?: string
  /** The commit a commit comparison showed. */
  commit?: string
  /** Its lines are numbered as in the index, which the agent reads from this flag. */
  staged?: boolean
  /** The file's version when the comment was made, to tell when the lines have moved under it. */
  version: string
  text: string
}

/** The author of a response the user wrote. */
export const USER_AUTHOR = 'you'

/** One response in the thread under an agent comment, oldest first. */
export interface ThreadResponse {
  id: string
  /** USER_AUTHOR, or the name of the agent that wrote it. */
  author: string
  text: string
  /** Milliseconds since the epoch. */
  at: number
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
 * rather than hive.db: they anchor to lines an agent is about to move, and belong
 * to this machine's sitting. A relaunch still must not lose what the user wrote.
 */
export const useChangesReview = defineStore('changesReview', () => {
  const sessions = useStorage<Record<string, ChangesComment[]>>('hive.changes.comments', {})
  const sent = useStorage<Record<string, SentReview[]>>('hive.changes.sent', {})
  // Keyed by the agent comment they answer: a response has no lines of its own and
  // goes out inside that comment on Send.
  const agentThreads = useStorage<Record<string, Record<string, readonly ThreadResponse[]>>>(
    'hive.changes.agentThreads',
    {},
  )

  function threads(sessionId: string): Readonly<Record<string, readonly ThreadResponse[]>> {
    return agentThreads.value[sessionId] ?? {}
  }

  function writeThread(sessionId: string, commentId: string, next: readonly ThreadResponse[]): void {
    const session = { ...threads(sessionId) }
    if (next.length) session[commentId] = next
    else delete session[commentId]
    const rest = { ...agentThreads.value }
    if (Object.keys(session).length) rest[sessionId] = session
    else delete rest[sessionId]
    agentThreads.value = rest
  }

  function thread(sessionId: string, commentId: string): readonly ThreadResponse[] {
    return threads(sessionId)[commentId] ?? []
  }

  /** Adds a response at the end of an agent comment's thread. Blank text adds nothing. */
  function respond(sessionId: string, commentId: string, text: string, author = USER_AUTHOR): void {
    if (!text.trim()) return
    writeThread(sessionId, commentId, [
      ...thread(sessionId, commentId),
      { id: crypto.randomUUID(), author, text: text.trim(), at: Date.now() },
    ])
  }

  /** Rewrites one response. Blank text removes it. */
  function editResponse(sessionId: string, commentId: string, responseId: string, text: string): void {
    writeThread(
      sessionId,
      commentId,
      thread(sessionId, commentId).flatMap((response) =>
        response.id !== responseId ? [response] : text.trim() ? [{ ...response, text: text.trim() }] : [],
      ),
    )
  }

  /**
   * Drops an agent comment's thread, or only the responses named in ids: a send
   * clears what it delivered and keeps a response written while it ran.
   */
  function clearThread(sessionId: string, commentId: string, ids?: readonly string[]): void {
    const keep = ids ? thread(sessionId, commentId).filter((response) => !ids.includes(response.id)) : []
    writeThread(sessionId, commentId, keep)
  }

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

  /**
   * Delivers the review and clears what it delivered. A failed delivery keeps every
   * comment, and a comment added or edited during the delivery stays to send next.
   * extra goes out with the review but is not held here: the agents' comments,
   * which their own store clears.
   * Returns why it failed, or null.
   */
  async function send(
    sessionId: string,
    pane: string,
    extra: readonly Omit<ChangesComment, 'id'>[] = [],
  ): Promise<string | null> {
    const delivered = [...comments(sessionId)]
    try {
      await SendChanges(sessionId, pane, [...delivered, ...extra.map((comment) => ({ ...comment, id: '' }))])
    } catch (err) {
      return errorText(err, 'Could not send the feedback.')
    }
    const deliveredText = new Map(delivered.map((comment) => [comment.id, comment.text]))
    write(
      sessionId,
      comments(sessionId).filter((comment) => deliveredText.get(comment.id) !== comment.text),
    )
    sent.value = {
      ...sent.value,
      [sessionId]: [
        { id: crypto.randomUUID(), sentAt: Date.now(), comments: delivered },
        ...sentReviews(sessionId),
      ].slice(0, MAX_SENT_REVIEWS),
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
    const restThreads = { ...agentThreads.value }
    delete restThreads[sessionId]
    agentThreads.value = restThreads
  }

  return {
    comments,
    add,
    update,
    remove,
    threads,
    thread,
    respond,
    editResponse,
    clearThread,
    targets,
    send,
    sentReviews,
    restore,
    copy,
    forget,
  }
})
