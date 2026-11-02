import { flushPromises } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { useTerminalAgentIcons } from '../useTerminalAgentIcons'

const mocks = vi.hoisted(() => ({ read: vi.fn(), persist: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.read,
  SetTerminalMonochromeAgentIcons: mocks.persist,
}))

beforeEach(() => {
  mocks.read.mockReset().mockResolvedValue({ terminalMonochromeAgentIcons: true })
  mocks.persist.mockReset().mockResolvedValue(undefined)
})

it('loads the saved choice and persists a change', async () => {
  const icons = useTerminalAgentIcons()
  expect(icons.monochrome.value).toBe(false)
  await flushPromises()
  expect(icons.monochrome.value).toBe(true)

  icons.setMonochrome(false)
  await flushPromises()
  expect(icons.monochrome.value).toBe(false)
  expect(mocks.persist).toHaveBeenLastCalledWith(false)
})
