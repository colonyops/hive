import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { InboxItem } from '../../types/feed'

const mocks = vi.hoisted(() => ({ SessionLaunchOptions: vi.fn(), CreateSession: vi.fn(), NewSessionDraft: vi.fn(), FailedSessionDraft: vi.fn(), DismissFailedSession: vi.fn(), SessionDraftFromActivity: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionLaunchOptions: mocks.SessionLaunchOptions,
  CreateSession: mocks.CreateSession,
  FailedSessionDraft: mocks.FailedSessionDraft,
  DismissFailedSession: mocks.DismissFailedSession,
  SessionDraftFromActivity: mocks.SessionDraftFromActivity,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice', () => ({ NewSessionDraft: mocks.NewSessionDraft }))

import { resetNewSessionForTests, useNewSession } from '../useNewSession'
import { resetToastsForTests, useToasts } from '../useToasts'

const options = { repositories: [], defaultRepository: 'https://github.com/hay-kot/hive-desktop.git', workspaces: [{ dir: 'alerts', name: 'Alerts', supportsPrompt: true }], agents: ['claude'], defaultAgent: 'claude' }
const item = { id: 7 } as InboxItem
const blank = { repository: options.defaultRepository, workspace: '', name: '', prompt: '', agent: 'claude' }

const failure = {
  reason: 'clone repository: git clone: exec git: exit status 1',
  step: 'Cloning repository...',
  output: 'Clone strategy: full\nCloning repository...',
  cloneStrategy: 'full',
  destination: '/home/u/.local/share/hive/repos/site-9fa2',
  leftoverCheckout: true,
  at: '2026-09-16T10:00:00Z',
}

// In the shape FailedSessionDraft answers with.
function pending(overrides: Record<string, unknown> = {}) {
  return { repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash', agent: 'pi', itemId: 0, failure, ...overrides }
}

beforeEach(() => {
  vi.clearAllMocks()
  resetToastsForTests()
  resetNewSessionForTests()
  mocks.SessionLaunchOptions.mockResolvedValue(options)
  mocks.FailedSessionDraft.mockResolvedValue({ repository: '', name: '', prompt: '' })
  mocks.DismissFailedSession.mockResolvedValue(undefined)
})

describe('useNewSession', () => {
  it('opens blank with the default repository', async () => {
    const s = useNewSession()
    await s.openBlank()
    expect(s.open.value).toBe(true)
    expect(s.initial.value).toEqual(blank)
  })

  it('prefers the repository of the session on screen', async () => {
    const s = useNewSession()
    await s.openBlank('https://github.com/acme/site.git')
    expect(s.initial.value).toEqual({ ...blank, repository: 'https://github.com/acme/site.git' })
  })

  it('opens the shared dialog on Chats when requested', async () => {
    const s = useNewSession()
    await s.openBlank('', 'workspace')
    expect(s.initialTarget.value).toBe('workspace')
  })

  it('falls back to the default when the session on screen has no remote', async () => {
    const s = useNewSession()
    await s.openBlank('')
    expect(s.initial.value.repository).toBe(options.defaultRepository)
  })

  it('prefills from an item draft', async () => {
    mocks.NewSessionDraft.mockResolvedValue({ repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash' })
    const s = useNewSession()
    await s.openFromItem(item)
    expect(mocks.NewSessionDraft).toHaveBeenCalledWith([7])
    expect(s.open.value).toBe(true)
    expect(s.initial.value).toEqual({ repository: 'acme/site', workspace: '', name: 'fix-crash', prompt: 'Fix the crash', agent: 'claude' })
  })

  it('opens an item draft on Chats when requested', async () => {
    mocks.NewSessionDraft.mockResolvedValue({ repository: 'acme/site', name: 'incident', prompt: 'Triage this alert' })
    const s = useNewSession()
    await s.openFromItem(item, 'workspace')
    expect(s.initialTarget.value).toBe('workspace')
  })

  it('creates one session for an ordered item selection', async () => {
    const items = [{ id: 7 }, { id: 11 }] as InboxItem[]
    mocks.NewSessionDraft.mockResolvedValue({ repository: 'acme/site', name: 'combined', prompt: 'Combined context' })
    mocks.CreateSession.mockResolvedValue(7)
    const s = useNewSession()

    await s.openFromItems(items)
    expect(mocks.NewSessionDraft).toHaveBeenCalledOnce()
    expect(mocks.NewSessionDraft).toHaveBeenCalledWith([7, 11])

    await s.submit({ repository: 'acme/site', name: 'combined', prompt: 'Combined context' })
    expect(mocks.CreateSession).toHaveBeenCalledOnce()
    expect(mocks.CreateSession).toHaveBeenCalledWith(expect.objectContaining({ itemIds: [7, 11] }))
  })

  it('leaves the repository unselected when a multi-item draft has no shared repository', async () => {
    mocks.NewSessionDraft.mockResolvedValue({ repository: '', name: 'combined', prompt: 'Combined context' })
    const s = useNewSession()

    await s.openFromItems([{ id: 7 }, { id: 11 }] as InboxItem[])

    expect(s.initial.value.repository).toBe('')
    expect(s.options.value?.defaultRepository).toBe('')
    expect(s.options.value?.repositories).toEqual(options.repositories)
  })

  it('reopens instantly from the cached options while the refresh is pending', async () => {
    const s = useNewSession()
    await s.openBlank()
    s.cancel()

    let release!: (opts: typeof options) => void
    mocks.SessionLaunchOptions.mockReturnValue(new Promise((resolve) => { release = resolve }))
    await s.openBlank()
    expect(s.open.value).toBe(true)
    expect(s.options.value).toEqual(options)

    release({ ...options, agents: ['claude', 'codex'] })
    await vi.waitFor(() => expect(s.options.value?.agents).toContain('codex'))
  })

  it('starts the session job and closes on submit', async () => {
    mocks.CreateSession.mockResolvedValue(7)
    const s = useNewSession()
    await s.openBlank()
    await s.submit({ repository: 'acme/site', name: 'fix-crash', prompt: 'go', agent: 'claude' })
    expect(mocks.CreateSession).toHaveBeenCalledWith({ repository: 'acme/site', workspace: '', name: 'fix-crash', prompt: 'go', agent: 'claude', itemIds: [] })
    expect(s.open.value).toBe(false)
    expect(useToasts().toasts.value.at(-1)?.message).toContain('fix-crash')
  })

  it('starts a workspace chat with the item context and target intact', async () => {
    mocks.NewSessionDraft.mockResolvedValue({ repository: 'acme/site', name: 'incident', prompt: 'Alert context' })
    mocks.CreateSession.mockResolvedValue(7)
    const s = useNewSession()
    await s.openFromItem(item)
    await s.submit({ workspace: 'alerts', name: 'incident', prompt: 'Alert context' })
    expect(mocks.CreateSession).toHaveBeenCalledWith({ repository: '', workspace: 'alerts', name: 'incident', prompt: 'Alert context', agent: '', itemIds: [7] })
  })

  // The session a form drafted from an item creates is recorded against that
  // item, so the detail pane can link through to it later.
  it('submits the item a draft came from, and nothing for a blank form', async () => {
    mocks.NewSessionDraft.mockResolvedValue({ repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash' })
    mocks.CreateSession.mockResolvedValue(7)
    const s = useNewSession()
    await s.openFromItem(item)
    await s.submit({ repository: 'acme/site', name: 'fix-crash', prompt: 'go' })
    expect(mocks.CreateSession).toHaveBeenCalledWith(expect.objectContaining({ itemIds: [7] }))

    await s.openBlank()
    await s.submit({ repository: 'acme/site', name: 'other', prompt: 'go' })
    expect(mocks.CreateSession).toHaveBeenLastCalledWith(expect.objectContaining({ itemIds: [] }))
  })

  it('restores a failed attempt instead of opening blank', async () => {
    mocks.FailedSessionDraft.mockResolvedValue(pending())
    const s = useNewSession()
    await s.openBlank('https://github.com/other/repo.git')
    expect(s.initial.value).toEqual({ repository: 'acme/site', workspace: '', name: 'fix-crash', prompt: 'Fix the crash', agent: 'pi' })
    expect(s.failure.value).toEqual(failure)
  })

  it('restores the failed attempt of the item whose form is opened, and only that item', async () => {
    mocks.NewSessionDraft.mockResolvedValue({ repository: 'acme/site', name: 'drafted', prompt: 'from the item' })
    mocks.FailedSessionDraft.mockResolvedValue(pending({ itemId: 7 }))
    const s = useNewSession()
    await s.openFromItem(item)
    expect(s.initial.value.name).toBe('fix-crash')
    expect(s.failure.value).toEqual(failure)

    s.cancel()
    mocks.FailedSessionDraft.mockResolvedValue(pending({ itemId: 99 }))
    await s.openFromItem(item)
    expect(s.initial.value.name).toBe('drafted')
    expect(s.failure.value).toBeNull()
  })

  it('raises a toast that does not time out, with the step and a retry', async () => {
    mocks.FailedSessionDraft.mockResolvedValue(pending())
    const s = useNewSession()
    await s.onCreateFailed()

    const toast = useToasts().toasts.value.at(-1)
    expect(toast?.severity).toBe('error')
    expect(toast?.message).toContain('fix-crash')
    expect(toast?.body).toContain('Cloning repository...')
    expect(toast?.body).toContain('exit status 1')
    expect(toast?.duration).toBe(0)
    expect(toast?.actions.map((a) => a.label)).toEqual(['Retry', 'Dismiss'])

    toast?.actions[0].onClick()
    await vi.waitFor(() => expect(s.open.value).toBe(true))
    expect(s.initial.value.name).toBe('fix-crash')
  })

  it('reopens the restored form over one that is already open', async () => {
    const s = useNewSession()
    await s.openBlank()
    const firstKey = s.formKey.value
    mocks.FailedSessionDraft.mockResolvedValue(pending())

    await s.openFailure()
    expect(s.initial.value.name).toBe('fix-crash')
    expect(s.formKey.value).toBeGreaterThan(firstKey)
  })

  it('drops the attempt the backend holds when the failure is dismissed', async () => {
    mocks.FailedSessionDraft.mockResolvedValue(pending())
    const s = useNewSession()
    await s.openBlank()
    expect(s.failure.value).not.toBeNull()

    await s.dismissFailure()
    expect(mocks.DismissFailedSession).toHaveBeenCalled()
    expect(s.failure.value).toBeNull()
  })

  it('clears the failure once a retry is accepted', async () => {
    mocks.FailedSessionDraft.mockResolvedValue(pending())
    mocks.CreateSession.mockResolvedValue(7)
    const s = useNewSession()
    await s.openBlank()
    await s.submit({ repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash' })
    expect(s.failure.value).toBeNull()
  })

  it('opens blank when the pending draft cannot be read', async () => {
    mocks.FailedSessionDraft.mockRejectedValue(new Error('unavailable'))
    const s = useNewSession()
    await s.openBlank()
    expect(s.open.value).toBe(true)
    expect(s.failure.value).toBeNull()
    expect(s.initial.value).toEqual(blank)
  })

  it('surfaces a validation error without closing', async () => {
    mocks.CreateSession.mockRejectedValue(new Error('session name is required'))
    const s = useNewSession()
    await s.openBlank()
    await s.submit({ repository: 'acme/site', name: '', prompt: '' })
    expect(s.open.value).toBe(true)
    expect(s.error.value).toContain('session name is required')
  })
})

// The durable path: the toast is gone and the process restarted, so the row is
// all that is left.
describe('useNewSession — retry from an activity row', () => {
  it('opens the form from a row and never reads the metadata itself', async () => {
    const metadata = { retry: 'session-create', repository: 'acme/site', name: 'fix-crash' }
    mocks.SessionDraftFromActivity.mockResolvedValue({
      repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash', agent: 'pi', itemId: 7,
      failure: { reason: 'exit status 1', step: 'Cloning repository...', output: '', cloneStrategy: 'full', at: '2026-09-16T10:00:00Z' },
    })
    const s = useNewSession()

    await s.openFromActivity(metadata)

    expect(mocks.SessionDraftFromActivity).toHaveBeenCalledWith(metadata)
    expect(s.open.value).toBe(true)
    expect(s.initial.value).toEqual({ repository: 'acme/site', workspace: '', name: 'fix-crash', prompt: 'Fix the crash', agent: 'pi' })
    expect(s.failure.value?.step).toBe('Cloning repository...')
  })

  // The row re-links the session to the item the form came from, so a retry
  // lands against the same inbox item as the attempt that failed.
  it('restores the item the row recorded', async () => {
    mocks.SessionDraftFromActivity.mockResolvedValue({ repository: 'acme/site', name: 'fix-crash', prompt: '', itemId: 7 })
    mocks.CreateSession.mockResolvedValue(7)
    const s = useNewSession()

    await s.openFromActivity({ retry: 'session-create' })
    await s.submit({ repository: 'acme/site', name: 'fix-crash', prompt: '' })

    expect(mocks.CreateSession).toHaveBeenCalledWith(expect.objectContaining({ itemIds: [7] }))
  })

  // The backend's own refusal is more use than the generic fallback, so it is
  // what the toast says.
  it('reports a row the backend refuses instead of opening an empty form', async () => {
    mocks.SessionDraftFromActivity.mockRejectedValue(new Error('this activity event carries no session to retry'))
    const s = useNewSession()

    await s.openFromActivity({ rule: 'auto-triage' })

    expect(s.open.value).toBe(false)
    expect(useToasts().toasts.value.at(-1)?.message).toBe('this activity event carries no session to retry')
    expect(useToasts().toasts.value.at(-1)?.severity).toBe('error')
  })
})
