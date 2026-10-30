import { ref, watch, onScopeDispose, type Ref } from 'vue'
import { SessionChanges as ReadChanges } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { SessionChanges } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { errorText } from '../lib/appError'
import { useWindowFocus } from '../stores/useWindowFocus'

const POLL_INTERVAL_MS = 5_000

export type DiffBase = 'branch' | 'uncommitted'

/**
 * The changed files in one session's checkout, for the Git pane. It reads only
 * while visible() holds and the window has focus, because a read costs several
 * git subprocesses. A failed read keeps the last good list beside its error.
 */
export function useSessionChanges(
  sessionId: Ref<string>,
  base: Ref<DiffBase>,
  visible: () => boolean,
): {
  changes: Ref<SessionChanges | null>
  error: Ref<string | null>
  refresh: () => Promise<void>
} {
  const changes = ref<SessionChanges | null>(null)
  const error = ref<string | null>(null)
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
      return
    }
    try {
      const next = await ReadChanges(id, base.value)
      if (current !== sequence) return
      changes.value = next
      error.value = null
    } catch (err) {
      if (current !== sequence) return
      error.value = errorText(err, 'Could not read the changes.')
    }
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
    [sessionId, base],
    ([id], [before]) => {
      // Another session's files for a frame would be a wrong fact, not a stale one.
      if (id !== before) changes.value = null
      error.value = null
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

  return { changes, error, refresh }
}
