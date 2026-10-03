import { beforeEach, describe, expect, it, vi } from 'vitest'
import { riskDescription, riskDetails, useSessionActions } from '../useSessionActions'
import { resetToastsForTests, useToasts } from '../useToasts'
import type { SessionSummary } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const mocks = vi.hoisted(() => ({
  DeleteSession: vi.fn(),
  PruneSessions: vi.fn(),
  RecycleSession: vi.fn(),
  RenameSession: vi.fn(),
  SessionDetail: vi.fn(),
  SessionRisk: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => mocks)

const session: SessionSummary = { id: 's1', name: 'review 81', slug: 'review-81', repo: 'acme/site', state: 'active' }

const noRisk = { uncommittedChanges: false, unpushedCommits: false, recycleDeletes: false }

beforeEach(() => {
  vi.clearAllMocks()
  resetToastsForTests()
  mocks.SessionRisk.mockResolvedValue(noRisk)
})

describe('riskDescription', () => {
  it('names the delete consequence', () => {
    expect(riskDescription('review 81', 'delete', noRisk)).toContain('Deleting review 81')
  })

  it('warns that recycling a worktree session deletes it', () => {
    const text = riskDescription('s', 'recycle', { ...noRisk, recycleDeletes: true })
    expect(text).toContain('deletes it')
    expect(text).toContain('git worktree')
  })

  it('describes a full-clone recycle as a reset', () => {
    expect(riskDescription('s', 'recycle', noRisk)).toContain('resets its clone')
  })
})

describe('riskDetails', () => {
  it('marks held work as danger lines', () => {
    const details = riskDetails({ uncommittedChanges: true, unpushedCommits: true, recycleDeletes: false })
    expect(details).toEqual([
      { tone: 'danger', text: expect.stringContaining('Uncommitted changes') },
      { tone: 'danger', text: expect.stringContaining('Unpushed commits') },
    ])
  })

  it('keeps a clean check on the list as an ok line rather than dropping it', () => {
    expect(riskDetails(noRisk).map((d) => d.tone)).toEqual(['ok', 'ok'])
  })

  it('mixes tones when only one check holds work', () => {
    expect(
      riskDetails({ uncommittedChanges: true, unpushedCommits: false, recycleDeletes: false }).map((d) => d.tone),
    ).toEqual(['danger', 'ok'])
    expect(
      riskDetails({ uncommittedChanges: false, unpushedCommits: true, recycleDeletes: false }).map((d) => d.tone),
    ).toEqual(['ok', 'danger'])
  })
})

describe('useSessionActions destructive operations', () => {
  it('confirms against the specific risk before deleting', async () => {
    mocks.SessionRisk.mockResolvedValue({ uncommittedChanges: true, unpushedCommits: false, recycleDeletes: false })
    const actions = useSessionActions()

    await actions.requestDelete(session)

    expect(mocks.SessionRisk).toHaveBeenCalledWith('s1')
    expect(actions.confirmation.options.value).not.toBeNull()
    expect(actions.confirmation.options.value?.details).toContainEqual({
      tone: 'danger',
      text: expect.stringContaining('Uncommitted changes'),
    })
    expect(mocks.DeleteSession).not.toHaveBeenCalled()

    await actions.confirmation.confirm()
    expect(mocks.DeleteSession).toHaveBeenCalledWith('s1')
    expect(actions.confirmation.options.value).toBeNull()
  })

  it('does not confirm at all when the pre-flight fails', async () => {
    mocks.SessionRisk.mockRejectedValue(new Error('no such session'))
    const actions = useSessionActions()

    await actions.requestDelete(session)

    expect(actions.confirmation.options.value).toBeNull()
    expect(mocks.DeleteSession).not.toHaveBeenCalled()
    expect(useToasts().toasts.value[0]?.message).toBe('no such session')
  })

  it('recycles through the same gate', async () => {
    const actions = useSessionActions()

    await actions.requestRecycle(session)
    await actions.confirmation.confirm()

    expect(mocks.RecycleSession).toHaveBeenCalledWith('s1')
  })

  it('names the count in the prune confirmation', async () => {
    const actions = useSessionActions()

    actions.requestPrune(3)
    expect(actions.confirmation.options.value?.description).toContain('All 3 recycled and corrupted sessions')

    await actions.confirmation.confirm()
    expect(mocks.PruneSessions).toHaveBeenCalled()
  })

  it('leaves the confirmation open with its error when the job cannot start', async () => {
    mocks.DeleteSession.mockRejectedValue(new Error('sessions are unavailable'))
    const actions = useSessionActions()

    await actions.requestDelete(session)
    await actions.confirmation.confirm()

    expect(actions.confirmation.options.value).not.toBeNull()
    expect(actions.confirmation.error.value).toBe('sessions are unavailable')
  })
})

describe('useSessionActions rename and read', () => {
  it('renames the session and asks the host to re-read the list', async () => {
    mocks.RenameSession.mockResolvedValue({ ...session, name: 'review 82', slug: 'review-82' })
    const onChanged = vi.fn()
    const actions = useSessionActions({ onChanged })

    actions.requestRename(session)
    expect(actions.renaming.value?.name).toBe('review 81')

    await actions.submitRename('  review 82  ')

    expect(mocks.RenameSession).toHaveBeenCalledWith('s1', 'review 82')
    // The re-read is how the new slug reaches the host: it is the tmux target.
    expect(onChanged).toHaveBeenCalled()
    expect(actions.renaming.value).toBeNull()
  })

  it('keeps the rename dialog open with the reason a name was rejected', async () => {
    mocks.RenameSession.mockRejectedValue(new Error('a session named "review 82" already exists'))
    const onChanged = vi.fn()
    const actions = useSessionActions({ onChanged })

    actions.requestRename(session)
    await actions.submitRename('review 82')

    expect(actions.renaming.value).not.toBeNull()
    expect(actions.renameError.value).toContain('already exists')
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('reads a session on demand for the detail view', async () => {
    mocks.SessionDetail.mockResolvedValue({ ...session, path: '/tmp/review-81' })
    const actions = useSessionActions()

    await actions.openDetail(session)
    expect(actions.detail.value?.path).toBe('/tmp/review-81')

    actions.closeDetail()
    expect(actions.detail.value).toBeNull()
  })
})
