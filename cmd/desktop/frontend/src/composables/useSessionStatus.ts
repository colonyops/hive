import { ref, watch, onScopeDispose, type Ref } from 'vue'
import {
  SessionGitStatus as ReadGitStatus,
  SessionPullRequest as ReadPullRequest,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  SessionGitStatus,
  SessionPullRequest,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { useWindowFocus } from './useWindowFocus'

// Git is four local subprocesses, so it can be polled. The pull request rides
// along on each poll and is answered from the Go-side cache, so this interval
// does not set how often the forge is asked.
const POLL_INTERVAL_MS = 15_000

/**
 * The attached session's checkout, for the session status bar. Scoped to one
 * session: a full read costs several git subprocesses.
 */
export function useSessionStatus(sessionId: Ref<string>): {
  git: Ref<SessionGitStatus | null>
  pullRequest: Ref<SessionPullRequest | null>
  /** Why the lookup failed, which is never the same as the branch having none. */
  pullRequestError: Ref<string>
  refresh: (options?: { refreshPullRequest?: boolean }) => Promise<void>
} {
  const git = ref<SessionGitStatus | null>(null)
  const pullRequest = ref<SessionPullRequest | null>(null)
  const pullRequestError = ref('')
  const { focused } = useWindowFocus()

  // An answer for the session you just left must not paint over the one you
  // switched to.
  let sequence = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  // The last answer per session, so switching back paints from memory instead
  // of blanking for the ~100ms a git read takes. Bounded by the sessions
  // visited in one run, which is small enough not to warrant eviction.
  const lastGit = new Map<string, SessionGitStatus>()
  const lastPullRequest = new Map<string, SessionPullRequest>()

  async function refresh(options: { refreshPullRequest?: boolean } = {}): Promise<void> {
    const id = sessionId.value
    const current = ++sequence
    if (!id) {
      git.value = null
      pullRequest.value = null
      pullRequestError.value = ''
      return
    }

    let status: SessionGitStatus
    try {
      status = await ReadGitStatus(id)
    } catch {
      // A transient failure must not blank a bar that had an answer.
      return
    }
    if (current !== sequence) return
    git.value = status
    lastGit.set(id, status)

    if (!status.resolved || !status.host || !status.owner || !status.repo || !status.branch) {
      pullRequest.value = null
      pullRequestError.value = ''
      lastPullRequest.delete(id)
      return
    }
    try {
      const pr = await ReadPullRequest(
        { host: status.host, owner: status.owner, repo: status.repo, branch: status.branch },
        options.refreshPullRequest ?? false,
      )
      if (current !== sequence) return
      pullRequest.value = pr
      pullRequestError.value = ''
      lastPullRequest.set(id, pr)
    } catch (error) {
      if (current !== sequence) return
      // Kept apart from a null pull request: the branch may well have one, and
      // saying otherwise is a claim this failure cannot support.
      pullRequestError.value = error instanceof Error ? error.message : String(error)
    }
  }

  function schedule(): void {
    clearTimeout(timer)
    timer = setTimeout(() => {
      void poll()
    }, POLL_INTERVAL_MS)
  }

  async function poll(): Promise<void> {
    if (focused.value) await refresh()
    schedule()
  }

  watch(
    sessionId,
    (id) => {
      // Seeded from the last answer for *this* session, never carried over from
      // the one being left: another session's branch for a frame would be a wrong
      // fact rather than a missing one.
      const remembered = id ? lastPullRequest.get(id) : undefined
      git.value = (id ? lastGit.get(id) : undefined) ?? null
      // Cached, so restoring it does not replay the arrival animation.
      pullRequest.value = remembered ? { ...remembered, cached: true } : null
      pullRequestError.value = ''
      void refresh()
      schedule()
    },
    { immediate: true },
  )

  // A blurred window's checkout keeps changing — an agent is committing in it —
  // so refocus is both the stalest moment and the one worth spending a request
  // to go behind the PR cache: you were probably just in a browser looking at
  // it. Focus is user-driven, so this cannot turn into polling the forge.
  watch(focused, (isFocused) => {
    if (isFocused) void refresh({ refreshPullRequest: true })
  })

  onScopeDispose(() => {
    sequence++
    clearTimeout(timer)
  })

  return { git, pullRequest, pullRequestError, refresh }
}
