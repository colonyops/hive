import { ref, watch, onScopeDispose, type Ref } from 'vue'
import {
  SessionBranches,
  SessionChanges as ReadChanges,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  DiffTarget,
  SessionChanges,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { errorText } from '../lib/appError'
import { hiveUiPlatform } from '../lib/hiveUiPlatform'
import { useWindowFocus } from '../stores/useWindowFocus'

const POLL_INTERVAL_MS = 5_000

export type DiffBase = 'branch' | 'uncommitted' | 'staged' | 'unstaged' | 'commit'

/** One string per comparison, for telling two targets apart. */
export function targetKey(target: DiffTarget): string {
  return `${target.base}:${target.against}:${target.commit}`
}

/**
 * The changed files in one session's checkout, for the Changes view. It reads only
 * while the window has focus, because a read costs several git subprocesses. A
 * failed read keeps the last good list beside its error.
 */
export function useSessionChanges(
  sessionId: Ref<string>,
  target: Ref<DiffTarget>,
): {
  changes: Ref<SessionChanges | null>
  error: Ref<string | null>
  /** The target moved and its answer has not landed, so changes still describes the last one. */
  pending: Ref<boolean>
} {
  const changes = ref<SessionChanges | null>(null)
  const error = ref<string | null>(null)
  const pending = ref(false)
  const { focused } = useWindowFocus()

  // An answer for the session or target you just left must not paint over the
  // one you switched to.
  let sequence = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false

  async function refresh(): Promise<void> {
    const id = sessionId.value
    const current = ++sequence
    if (!id) {
      changes.value = null
      error.value = null
      pending.value = false
      return
    }
    try {
      const next = await ReadChanges(id, target.value)
      if (current !== sequence) return
      changes.value = next
      error.value = null
    } catch (err) {
      if (current !== sequence) return
      error.value = errorText(err, 'Could not read the changes.')
      // The first read for a target has no list of its own, and the last target's
      // would be read and commented on as if it were this one's.
      if (pending.value) changes.value = null
    }
    pending.value = false
  }

  function schedule(): void {
    clearTimeout(timer)
    timer = setTimeout(() => {
      void poll()
    }, POLL_INTERVAL_MS)
  }

  async function poll(): Promise<void> {
    if (focused.value) await refresh()
    // Disposal during that read found no timer to clear, so only this stops the loop.
    if (!disposed) schedule()
  }

  watch(
    [sessionId, () => targetKey(target.value)],
    ([id], [before]) => {
      // Another session's files for a frame would be a wrong fact, not a stale one.
      if (id !== before) changes.value = null
      error.value = null
      pending.value = true
      void refresh()
      schedule()
    },
    { immediate: true },
  )

  watch(focused, (reading) => {
    if (reading) void refresh()
  })

  onScopeDispose(() => {
    disposed = true
    sequence++
    clearTimeout(timer)
  })

  return { changes, error, pending }
}

/**
 * The full refs a session's checkout can compare against, read when the session
 * changes. A failure leaves only the default branch on offer.
 */
export function useSessionBranches(sessionId: Ref<string>): Ref<string[]> {
  const branches = ref<string[]>([])
  const log = hiveUiPlatform.log('changes')

  watch(
    sessionId,
    async (id) => {
      branches.value = []
      if (!id) return
      try {
        const refs = (await SessionBranches(id)) ?? []
        if (id === sessionId.value) branches.value = refs
      } catch (err) {
        log.warn('could not list branches', { err })
      }
    },
    { immediate: true },
  )

  return branches
}
