import { computed, shallowRef } from 'vue'
import { useWailsEvent } from '../composables/useWailsEvent'
import { driveEventAuthor, type DriveAuthor } from '../lib/drive'
import { defineStore } from './defineStore'

interface Write {
  space: string
  path: string
}

/**
 * What agents wrote while the app ran, kept outside any pane so a write in the
 * background still counts when the pane opens later. Two things live here:
 * the unseen dot, keyed by author (a chat or a hive session) so a switch
 * never inherits another's dot, and each author's last write per space, which
 * a pane opens by default because the tree carries no authors.
 *
 * This store is the only writer of the unseen set. A pane showing an author's
 * documents registers that author while it is on screen, and a write by a
 * registered author is already seen. Deciding it here rather than in each
 * area keeps a hidden area from marking a write another area is showing.
 */
export const useDriveActivity = defineStore('driveActivity', () => {
  const unseen = shallowRef<ReadonlySet<DriveAuthor>>(new Set())
  const lastWrites = shallowRef<ReadonlyMap<string, Write>>(new Map())
  const unopened = shallowRef<ReadonlySet<string>>(new Set())
  const visible = new Map<DriveAuthor, number>()

  const writeKey = (author: DriveAuthor, space: string) => `${author}\n${space}`
  const docKey = (space: string, path: string) => `${space}\n${path}`

  useWailsEvent('drive:updated', (event) => {
    const author = driveEventAuthor(event.data)
    if (author !== null && !visible.has(author)) markUnseen(author)
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as
      { space?: unknown; paths?: unknown } | undefined
    const space = typeof payload?.space === 'string' ? payload.space : ''
    const paths = Array.isArray(payload?.paths)
      ? payload.paths.filter((path): path is string => typeof path === 'string' && path !== '')
      : []
    if (author === null || !space || !paths.length) return
    const writes = new Map(lastWrites.value)
    writes.set(writeKey(author, space), { space, path: paths[paths.length - 1] })
    lastWrites.value = writes
    const docs = new Set(unopened.value)
    for (const path of paths) docs.add(docKey(space, path))
    unopened.value = docs
  })

  function markUnseen(author: DriveAuthor): void {
    if (!author || unseen.value.has(author)) return
    unseen.value = new Set(unseen.value).add(author)
  }

  function clearUnseen(author: DriveAuthor): void {
    if (!unseen.value.has(author)) return
    const next = new Set(unseen.value)
    next.delete(author)
    unseen.value = next
  }

  /** A pane shows the author's documents from now on, which also reads its dot. */
  function registerVisible(author: DriveAuthor): void {
    if (!author) return
    visible.set(author, (visible.get(author) ?? 0) + 1)
    clearUnseen(author)
  }

  function unregisterVisible(author: DriveAuthor): void {
    const count = visible.get(author)
    if (count === undefined) return
    if (count > 1) visible.set(author, count - 1)
    else visible.delete(author)
  }

  function isUnseen(author: DriveAuthor | null): boolean {
    return author !== null && unseen.value.has(author)
  }

  function lastWrite(author: DriveAuthor | null, space: string): string | null {
    if (author === null) return null
    return lastWrites.value.get(writeKey(author, space))?.path ?? null
  }

  /** An agent wrote the document and nobody has opened it since. */
  function isUnopened(space: string, path: string): boolean {
    return unopened.value.has(docKey(space, path))
  }

  function markOpened(space: string, path: string): void {
    const key = docKey(space, path)
    if (!unopened.value.has(key)) return
    const next = new Set(unopened.value)
    next.delete(key)
    unopened.value = next
  }

  return {
    unseen: computed(() => unseen.value),
    registerVisible,
    unregisterVisible,
    isUnseen,
    lastWrite,
    isUnopened,
    markOpened,
  }
})
