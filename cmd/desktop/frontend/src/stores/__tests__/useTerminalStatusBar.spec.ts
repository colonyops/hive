import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTerminalStatusBar } from '../useTerminalStatusBar'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTerminalShowStatusBar: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetTerminalShowStatusBar: mocks.SetTerminalShowStatusBar,
}))

describe('useTerminalStatusBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ terminalShowStatusBar: false })
    mocks.SetTerminalShowStatusBar.mockResolvedValue(undefined)
  })

  it('shows the bar until the stored setting says otherwise', async () => {
    const { showStatusBar } = useTerminalStatusBar()
    expect(showStatusBar.value).toBe(true)

    await flushPromises()

    expect(showStatusBar.value).toBe(false)
  })

  it('applies a toggle at once and persists it', async () => {
    const { showStatusBar, setShowStatusBar } = useTerminalStatusBar()
    await flushPromises()

    setShowStatusBar(true)
    expect(showStatusBar.value).toBe(true)
    await flushPromises()

    expect(mocks.SetTerminalShowStatusBar).toHaveBeenCalledWith(true)
  })
})
