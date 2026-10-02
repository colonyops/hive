import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTerminalShowWindows } from '../useTerminalShowWindows'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTerminalShowWindows: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetTerminalShowWindows: mocks.SetTerminalShowWindows,
}))

describe('useTerminalShowWindows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ terminalShowWindows: false })
    mocks.SetTerminalShowWindows.mockResolvedValue(undefined)
  })

  it('shows windows until the stored setting says otherwise, then reports ready', async () => {
    const { showWindows, ready } = useTerminalShowWindows()
    expect(showWindows.value).toBe(true)
    expect(ready.value).toBe(false)

    await flushPromises()

    expect(showWindows.value).toBe(false)
    expect(ready.value).toBe(true)
  })

  it('applies a toggle at once and persists it', async () => {
    const { showWindows, setShowWindows } = useTerminalShowWindows()
    await flushPromises()

    setShowWindows(true)
    expect(showWindows.value).toBe(true)
    await flushPromises()

    expect(mocks.SetTerminalShowWindows).toHaveBeenCalledWith(true)
  })
})
