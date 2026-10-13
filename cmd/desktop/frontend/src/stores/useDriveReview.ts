import { computed, shallowRef } from 'vue'
import {
  AddComment,
  Clear,
  DeleteComment,
  Feedback,
  Review,
  Send,
  UpdateComment,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice'
import type { DocumentReviewWire } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { AgentTargetRef } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { errorText } from '../lib/appError'
import type { SourceRange } from '../lib/driveReviewSelection'
import { defineStore } from './defineStore'
import { useResource, type Resource } from './useResource'

const emptyReview: DocumentReviewWire = { sessionId: '', current: true, comments: [] }

/** An action's outcome: null, or the text of why it failed. */
export type ReviewResult = string | null

/**
 * Comments on Drive Markdown documents, one review per space and path, read
 * the first time a document asks. The general note is local until it is
 * copied or sent.
 */
export const useDriveReview = defineStore('driveReview', () => {
  const reviews = new Map<string, Resource<DocumentReviewWire>>()
  const notes = shallowRef<ReadonlyMap<string, string>>(new Map())
  // The comment the user last pointed at, from its card or its block, so the
  // reader and the sidebar highlight the same one.
  const active = shallowRef<string | null>(null)

  function setActive(id: string | null): void {
    active.value = id
  }

  const keyOf = (space: string, path: string) => `${space}\n${path}`

  function resource(space: string, path: string): Resource<DocumentReviewWire> {
    const key = keyOf(space, path)
    let entry = reviews.get(key)
    if (!entry) {
      entry = useResource(async () => (await Review(space, path)) ?? emptyReview, {
        initial: emptyReview,
        errorFallback: `Could not read the comments on ${path}.`,
      })
      reviews.set(key, entry)
      void entry.reload()
    }
    return entry
  }

  /** The review of one document, loaded on first use. */
  function review(space: string, path: string) {
    const entry = resource(space, path)
    return {
      data: computed(() => entry.data.value),
      error: entry.error,
      loaded: entry.loaded,
    }
  }

  async function act(
    space: string,
    path: string,
    run: () => Promise<unknown>,
    fallback: string,
  ): Promise<ReviewResult> {
    try {
      await run()
      return null
    } catch (err) {
      return errorText(err, fallback)
    } finally {
      await resource(space, path).reload()
    }
  }

  function add(space: string, path: string, range: SourceRange, text: string): Promise<ReviewResult> {
    return act(
      space,
      path,
      () =>
        AddComment(space, path, { startLine: range.startLine, endLine: range.endLine, context: range.context, text }),
      'Could not save the comment.',
    )
  }

  function update(space: string, path: string, id: string, text: string): Promise<ReviewResult> {
    return act(space, path, () => UpdateComment(space, path, id, text), 'Could not update the comment.')
  }

  function remove(space: string, path: string, id: string): Promise<ReviewResult> {
    return act(space, path, () => DeleteComment(space, path, id), 'Could not delete the comment.')
  }

  /** Whether a document has comments or a note in progress, which is when its sidebar shows. */
  function inReview(space: string, path: string): boolean {
    return (resource(space, path).data.value.comments?.length ?? 0) > 0 || note(space, path) !== ''
  }

  function note(space: string, path: string): string {
    return notes.value.get(keyOf(space, path)) ?? ''
  }

  function setNote(space: string, path: string, text: string): void {
    const next = new Map(notes.value)
    if (text) next.set(keyOf(space, path), text)
    else next.delete(keyOf(space, path))
    notes.value = next
  }

  /** The feedback text for the document's comments and note. Rejects when Go refuses. */
  async function feedback(space: string, path: string): Promise<string> {
    return (await Feedback(space, path, note(space, path))) ?? ''
  }

  async function clear(space: string, path: string): Promise<ReviewResult> {
    const result = await act(space, path, () => Clear(space, path), 'Could not clear the comments.')
    if (!result) setNote(space, path, '')
    return result
  }

  async function send(space: string, path: string, to: AgentTargetRef): Promise<ReviewResult> {
    const result = await act(
      space,
      path,
      () => Send(space, path, note(space, path), to),
      'Could not send the feedback.',
    )
    if (!result) setNote(space, path, '')
    return result
  }

  // A rewrite of a document under review turns its comments stale; the event
  // names the paths it changed, a space with no paths, or nothing at all.
  useWailsEvent('drive:updated', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as
      { space?: unknown; paths?: unknown } | undefined
    const space = typeof payload?.space === 'string' ? payload.space : ''
    const paths = Array.isArray(payload?.paths)
      ? payload.paths.filter((path): path is string => typeof path === 'string' && path !== '')
      : []
    for (const [key, entry] of reviews) {
      const [entrySpace, entryPath] = key.split('\n')
      if (!space || (entrySpace === space && (!paths.length || paths.includes(entryPath)))) void entry.reload()
    }
  })

  return {
    active: computed(() => active.value),
    setActive,
    review,
    inReview,
    note,
    setNote,
    add,
    update,
    remove,
    feedback,
    clear,
    send,
  }
})
