import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useSessionStatus } from '../useSessionStatus'

const mocks = vi.hoisted(() => ({ SessionGitStatus: vi.fn(), SessionPullRequest: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionGitStatus: mocks.SessionGitStatus,
  SessionPullRequest: mocks.SessionPullRequest,
}))
vi.mock('../useWindowFocus', () => ({ useWindowFocus: () => ({ focused: ref(true) }) }))

function gitStatus(branch: string) {
  return {
    path: `/tmp/${branch}`, branch, dirty: false, unpushed: false, additions: 0, deletions: 0,
    host: 'github.com', owner: 'acme', repo: 'site', resolved: true, error: '',
  }
}

/** Runs body inside a scope so the composable's onScopeDispose timer is cleaned up. */
async function withStatus(sessionId: ReturnType<typeof ref<string>>, body: (status: ReturnType<typeof useSessionStatus>) => Promise<void>) {
  const scope = effectScope()
  const status = scope.run(() => useSessionStatus(sessionId as never))!
  try {
    await body(status)
  } finally {
    scope.stop()
  }
}

describe('useSessionStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.SessionGitStatus.mockImplementation(async (id: string) => gitStatus(id === '1' ? 'feat/one' : 'feat/two'))
    mocks.SessionPullRequest.mockResolvedValue({ status: 'none' })
  })

  // A git read takes long enough that blanking on every switch is visible.
  it('seeds a revisited session from its last answer instead of blanking', async () => {
    const sessionId = ref('1')
    await withStatus(sessionId, async (status) => {
      await vi.waitFor(() => expect(status.git.value?.branch).toBe('feat/one'))

      sessionId.value = '2'
      await vi.waitFor(() => expect(status.git.value?.branch).toBe('feat/two'))

      // Back again, checked before the git read can resolve.
      sessionId.value = '1'
      await nextTick()
      expect(status.git.value?.branch).toBe('feat/one')
    })
  })

  // Showing the outgoing session's branch for a frame would be worse than the
  // blank it replaces — it would be a wrong fact rather than a missing one.
  it('never carries one session’s status onto another', async () => {
    const sessionId = ref('1')
    await withStatus(sessionId, async (status) => {
      await vi.waitFor(() => expect(status.git.value?.branch).toBe('feat/one'))

      // Never resolves, so the only thing that could be on screen is whatever
      // the switch itself put there.
      mocks.SessionGitStatus.mockImplementation(() => new Promise(() => {}))
      sessionId.value = '3'
      await nextTick()
      expect(status.git.value).toBeNull()
    })
  })

  // A restored pull request was known before the paint, so it must not replay
  // the arrival animation, which is gated on exactly this flag.
  it('marks a restored pull request as cached', async () => {
    mocks.SessionPullRequest.mockResolvedValue({ status: 'found', number: 7, cached: false })
    const sessionId = ref('1')
    await withStatus(sessionId, async (status) => {
      await vi.waitFor(() => expect(status.pullRequest.value?.number).toBe(7))
      expect(status.pullRequest.value?.cached).toBe(false)

      sessionId.value = '2'
      await vi.waitFor(() => expect(status.git.value?.branch).toBe('feat/two'))
      sessionId.value = '1'
      await nextTick()

      expect(status.pullRequest.value?.number).toBe(7)
      expect(status.pullRequest.value?.cached).toBe(true)
    })
  })
})
