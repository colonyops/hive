import { useStorage } from '@vueuse/core'
import { errorText } from '../lib/appError'
import type { DiffSpan } from '../lib/diffReview'
import {
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
  /** The file's version when the comment was made, to tell when the lines have moved under it. */
  version: string
  text: string
}

/**
 * Comments on a session's diffs, waiting to go to its agent. In browser storage
 * rather than hive.db: they anchor to lines an agent is about to move, are gone
 * the moment they are sent, and belong to this machine's sitting. A relaunch
 * still must not lose what the user wrote.
 */
export const useChangesReview = defineStore('changesReview', () => {
  const sessions = useStorage<Record<string, ChangesComment[]>>('hive.changes.comments', {})

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
    write(sessionId, [])
    return null
  }

  return { comments, add, update, remove, targets, send }
})
