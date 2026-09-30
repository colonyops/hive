import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useFirstRun } from '../useFirstRun'

const mocks = vi.hoisted(() => ({
  OnboardingSettings: vi.fn(),
  SetOnboardingCompleted: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  OnboardingSettings: mocks.OnboardingSettings,
  SetOnboardingCompleted: mocks.SetOnboardingCompleted,
}))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.SetOnboardingCompleted.mockResolvedValue(undefined)
})

describe('useFirstRun', () => {
  it('is unknown until the read lands, then reports the marker', async () => {
    mocks.OnboardingSettings.mockResolvedValue({ completed: false })
    const firstRun = useFirstRun()
    expect(firstRun.completed.value).toBeNull()

    await firstRun.load()
    expect(firstRun.completed.value).toBe(false)
  })

  // The shell must not be held behind a walk it cannot know it owes.
  it('treats a failed read as completed', async () => {
    mocks.OnboardingSettings.mockRejectedValue(new Error('settings unreadable'))
    const firstRun = useFirstRun()

    await firstRun.load()

    expect(firstRun.completed.value).toBe(true)
  })

  it('flips the flag before the write, so the walk ends even if the write fails', async () => {
    mocks.OnboardingSettings.mockResolvedValue({ completed: false })
    mocks.SetOnboardingCompleted.mockRejectedValue(new Error('disk full'))
    const firstRun = useFirstRun()
    await firstRun.load()

    await firstRun.complete()

    expect(mocks.SetOnboardingCompleted).toHaveBeenCalledOnce()
    expect(firstRun.completed.value).toBe(true)
  })
})
