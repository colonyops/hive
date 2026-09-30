import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  Pending: vi.fn(),
  Acknowledge: vi.fn(),
  History: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/releasenotesservice', () => ({
  Pending: mocks.Pending,
  Acknowledge: mocks.Acknowledge,
  History: mocks.History,
}))

function note(version: string, summary = '') {
  return { version, date: '2026-08-06', summary, body: `notes for ${version}`, draft: false }
}

function draft(summary = '') {
  return { version: '', date: '', summary, body: 'unreleased work', draft: true }
}

async function loadComposable() {
  const { useReleaseNotes } = await import('../useReleaseNotes')
  const { useToasts } = await import('../useToasts')
  return { notes: useReleaseNotes(), toasts: useToasts() }
}

describe('useReleaseNotes', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    mocks.Acknowledge.mockResolvedValue(undefined)
    mocks.History.mockResolvedValue([])
  })

  it('shows nothing when the backend reports no pending notes', async () => {
    mocks.Pending.mockResolvedValue({ show: false, presentation: '', version: '', entries: null })
    const { notes, toasts } = await loadComposable()

    await notes.checkOnLaunch()

    expect(notes.dialogOpen.value).toBe(false)
    expect(toasts.toasts.value).toHaveLength(0)
    expect(mocks.Acknowledge).not.toHaveBeenCalled()
  })

  it('opens the dialog for a modal presentation and acknowledges only on dismiss', async () => {
    mocks.Pending.mockResolvedValue({
      show: true, presentation: 'modal', version: '1.3.0', entries: [note('1.3.0')],
    })
    const { notes, toasts } = await loadComposable()

    await notes.checkOnLaunch()
    expect(notes.dialogOpen.value).toBe(true)
    expect(notes.pendingVersion.value).toBe('1.3.0')
    expect(toasts.toasts.value).toHaveLength(0)
    expect(mocks.Acknowledge).not.toHaveBeenCalled()

    await notes.dismiss()
    expect(notes.dialogOpen.value).toBe(false)
    expect(mocks.Acknowledge).toHaveBeenCalledTimes(1)
  })

  // The toast is the notification, so it counts as seen the moment it appears —
  // there is nothing for the user to dismiss. A prerelease bump carries the
  // draft, and the draft's summary is what it has to say.
  it('raises a toast for a toast presentation and acknowledges immediately', async () => {
    mocks.Pending.mockResolvedValue({
      show: true,
      presentation: 'toast',
      version: '1.3.0-dev.4',
      entries: [draft('Terminal fixes.')],
    })
    const { notes, toasts } = await loadComposable()

    await notes.checkOnLaunch()

    expect(notes.dialogOpen.value).toBe(false)
    expect(toasts.toasts.value).toHaveLength(1)
    expect(toasts.toasts.value[0].message).toBe('Updated to 1.3.0-dev.4')
    expect(toasts.toasts.value[0].body).toBe('Terminal fixes.')
    expect(mocks.Acknowledge).toHaveBeenCalledTimes(1)
  })

  it("opens the dialog from the toast's action", async () => {
    mocks.Pending.mockResolvedValue({
      show: true, presentation: 'toast', version: '1.3.0-dev.4', entries: [draft()],
    })
    const { notes, toasts } = await loadComposable()

    await notes.checkOnLaunch()
    toasts.toasts.value[0].actions[0].onClick()

    expect(notes.dialogOpen.value).toBe(true)
  })

  it('stays silent when the backend call fails', async () => {
    mocks.Pending.mockRejectedValue(new Error('unavailable'))
    const { notes, toasts } = await loadComposable()

    await expect(notes.checkOnLaunch()).resolves.toBeUndefined()
    expect(notes.dialogOpen.value).toBe(false)
    expect(toasts.toasts.value).toHaveLength(0)
  })

  it('returns an empty history rather than throwing when the backend fails', async () => {
    mocks.History.mockRejectedValue(new Error('unavailable'))
    const { notes } = await loadComposable()

    await expect(notes.loadHistory()).resolves.toEqual([])
  })
})
