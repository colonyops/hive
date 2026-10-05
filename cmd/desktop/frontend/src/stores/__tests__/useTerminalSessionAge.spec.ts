import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defaultTerminalSessionAgeThresholdDays, sessionAgeDays, useTerminalSessionAge } from '../useTerminalSessionAge'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTerminalSessionAge: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetTerminalSessionAge: mocks.SetTerminalSessionAge,
}))

describe('sessionAgeDays', () => {
  const now = Date.parse('2026-10-10T12:00:00Z')

  it('returns elapsed whole days once the threshold is reached', () => {
    expect(sessionAgeDays('2026-10-05T12:00:00Z', 5, now)).toBe(5)
    expect(sessionAgeDays('2026-10-04T00:00:00Z', 5, now)).toBe(6)
  })

  it('hides recent, invalid, and future timestamps', () => {
    expect(sessionAgeDays('2026-10-06T12:00:00Z', 5, now)).toBeNull()
    expect(sessionAgeDays('not-a-time', 5, now)).toBeNull()
    expect(sessionAgeDays('2026-10-11T12:00:00Z', 5, now)).toBeNull()
  })
})

describe('useTerminalSessionAge', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({
      terminalShowSessionAge: false,
      terminalSessionAgeThresholdDays: defaultTerminalSessionAgeThresholdDays,
    })
    mocks.SetTerminalSessionAge.mockResolvedValue(undefined)
  })

  it('persists the toggle and threshold together', async () => {
    const setting = useTerminalSessionAge()
    await flushPromises()

    setting.setEnabled(true)
    setting.setThresholdDays(7)
    await flushPromises()

    expect(setting.enabled.value).toBe(true)
    expect(setting.thresholdDays.value).toBe(7)
    expect(mocks.SetTerminalSessionAge.mock.calls).toEqual([
      [true, defaultTerminalSessionAgeThresholdDays],
      [true, 7],
    ])
  })
})
