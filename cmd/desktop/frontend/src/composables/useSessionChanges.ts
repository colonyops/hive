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
 * The changed files in one session's checkout, for the Git pane. It reads only
 * while visible() holds and the window has focus, because a read costs several
 * git subprocesses. A failed read keeps the last good list beside its error.
 */
export function useSessionChanges(
  sessionId: Ref<string>,
  target: Ref<DiffTarget>,
  visible: () => boolean,
): {
  changes: Ref<SessionChanges | null>
  error: Ref<string | null>
  /** The target moved and its answer has not landed, so changes still describes the last one. */
  pending: Ref<boolean>
  refresh: () => Promise<void>
} {
  const changes = ref<SessionChanges | null>(null)
  const error = ref<string | null>(null)
  const pending = ref(false)
  const { focused } = useWindowFocus()

  // An answer for the session or base you just left must not paint over the
  // one you switched to.
  let sequence = 0
  let timer: ReturnType<typeof setTimeout> | undefined

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
    if (visible() && focused.value) await refresh()
    schedule()
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

  watch(
    () => visible() && focused.value,
    (reading) => {
      if (reading) void refresh()
    },
  )

  onScopeDispose(() => {
    sequence++
    clearTimeout(timer)
  })

  return { changes, error, pending, refresh }
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
