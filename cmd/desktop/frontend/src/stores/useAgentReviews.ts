import { shallowRef } from 'vue'
import { errorText } from '../lib/appError'
import { hiveUiPlatform } from '../lib/hiveUiPlatform'
import {
  DiffReviews,
  DismissDiffComment,
  DismissDiffReview,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice'
import type { DiffReviewWire } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'

/**
 * Agents' reviews of a session's branch diff, read from hive.db. Any agent
 * writes them, from the desktop or the hive CLI, so a session's reviews are
 * re-read whenever Go says one changed, for as long as a view watches it.
 */
export const useAgentReviews = defineStore('agentReviews', () => {
  const log = hiveUiPlatform.log('agent-reviews')
  const bySession = shallowRef<Readonly<Record<string, readonly DiffReviewWire[]>>>({})
  const errors = shallowRef<Readonly<Record<string, string>>>({})
  const watchers = new Map<string, number>()

  function set<T>(map: Readonly<Record<string, T>>, id: string, value: T | undefined): Record<string, T> {
    const next = { ...map }
    if (value === undefined) delete next[id]
    else next[id] = value
    return next
  }

  async function reload(sessionId: string): Promise<void> {
    try {
      const list = (await DiffReviews(sessionId)) ?? []
      bySession.value = set(bySession.value, sessionId, list)
      errors.value = set(errors.value, sessionId, undefined)
    } catch (err) {
      const message = errorText(err, 'Could not read the agent reviews.')
      log.warn('could not read the agent reviews', { sessionId, message })
      errors.value = set(errors.value, sessionId, message)
    }
  }

  /** Keeps a session's reviews current until the returned stop is called. */
  function watch(sessionId: string): () => void {
    watchers.set(sessionId, (watchers.get(sessionId) ?? 0) + 1)
    void reload(sessionId)
    let stopped = false
    return () => {
      if (stopped) return
      stopped = true
      const left = (watchers.get(sessionId) ?? 1) - 1
      if (left > 0) watchers.set(sessionId, left)
      else watchers.delete(sessionId)
    }
  }

  // The event comes from a fingerprint of every review, so it cannot say whose changed.
  useWailsEvent('diff-reviews:updated', () => {
    for (const watched of watchers.keys()) void reload(watched)
  })

  function reviews(sessionId: string): readonly DiffReviewWire[] {
    return bySession.value[sessionId] ?? []
  }

  // The row leaves at once: the walkthrough moves on without waiting for Go. A failed
  // call reads the session again, which puts it back.
  function drop(sessionId: string, keep: (review: DiffReviewWire) => DiffReviewWire | null): void {
    bySession.value = set(
      bySession.value,
      sessionId,
      reviews(sessionId).flatMap((review) => keep(review) ?? []),
    )
  }

  /** Removes one comment. Returns why it could not, or null. */
  async function dismiss(sessionId: string, commentId: string): Promise<string | null> {
    drop(sessionId, (review) => {
      const comments = (review.comments ?? []).filter((comment) => comment.id !== commentId)
      return comments.length ? { ...review, comments } : null
    })
    try {
      await DismissDiffComment(commentId)
      return null
    } catch (err) {
      void reload(sessionId)
      return errorText(err, 'Could not dismiss the comment.')
    }
  }

  /** Removes a whole review. Returns why it could not, or null. */
  async function dismissReview(sessionId: string, reviewId: string): Promise<string | null> {
    drop(sessionId, (review) => (review.id === reviewId ? null : review))
    try {
      await DismissDiffReview(reviewId)
      return null
    } catch (err) {
      void reload(sessionId)
      return errorText(err, 'Could not dismiss the review.')
    }
  }

  function error(sessionId: string): string | null {
    return errors.value[sessionId] ?? null
  }

  return { reviews, error, watch, dismiss, dismissReview }
})
