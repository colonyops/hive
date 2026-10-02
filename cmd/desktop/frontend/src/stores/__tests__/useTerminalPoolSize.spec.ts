import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTerminalPoolSize } from '../useTerminalPoolSize'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTerminalPoolSize: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetTerminalPoolSize: mocks.SetTerminalPoolSize,
}))

describe('useTerminalPoolSize', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ terminalPoolSize: 5 })
    mocks.SetTerminalPoolSize.mockResolvedValue(undefined)
  })

  it('hydrates the stored size', async () => {
    const { poolSize } = useTerminalPoolSize()
    expect(poolSize.value).toBe(3)

    await flushPromises()

    expect(poolSize.value).toBe(5)
  })

  it('falls back to the default for a size outside the ladder', async () => {
    mocks.AppearanceSettings.mockResolvedValue({ terminalPoolSize: 12 })

    const { poolSize } = useTerminalPoolSize()
    await flushPromises()

    expect(poolSize.value).toBe(3)
  })

  it('applies a change at once, resolves it onto the ladder, and persists it', async () => {
    const { poolSize, setPoolSize } = useTerminalPoolSize()
    await flushPromises()

    setPoolSize(2)
    expect(poolSize.value).toBe(2)
    setPoolSize(99)
    expect(poolSize.value).toBe(3)
    await flushPromises()

    expect(mocks.SetTerminalPoolSize.mock.calls).toEqual([[2], [3]])
  })
})
