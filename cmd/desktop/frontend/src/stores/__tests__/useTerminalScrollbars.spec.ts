import { flushPromises } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { useTerminalScrollbars } from '../useTerminalScrollbars'

const mocks = vi.hoisted(() => ({ read: vi.fn(), persist: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.read,
  SetTerminalShowScrollbars: mocks.persist,
}))

beforeEach(() => {
  mocks.read.mockReset().mockResolvedValue({ terminalShowScrollbars: false })
  mocks.persist.mockReset().mockResolvedValue(undefined)
})

it('hydrates the saved preference once for every mounted consumer', async () => {
  mocks.read.mockResolvedValue({ terminalShowScrollbars: true })
  const first = useTerminalScrollbars()
  const second = useTerminalScrollbars()
  expect(first.showScrollbars.value).toBe(false)
  await flushPromises()
  expect(first.showScrollbars).toBe(second.showScrollbars)
  expect(second.showScrollbars.value).toBe(true)
  expect(mocks.read).toHaveBeenCalledTimes(1)
})

it('preserves a toggle made during hydration and serializes rapid writes', async () => {
  let loaded!: (value: { terminalShowScrollbars: boolean }) => void
  let saved!: () => void
  mocks.read.mockImplementation(
    () =>
      new Promise((resolve) => {
        loaded = resolve
      }),
  )
  mocks.persist.mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        saved = resolve
      }),
  )
  const { showScrollbars, setShowScrollbars } = useTerminalScrollbars()
  setShowScrollbars(true)
  loaded({ terminalShowScrollbars: false })
  await flushPromises()
  expect(showScrollbars.value).toBe(true)
  setShowScrollbars(false)
  await flushPromises()
  expect(mocks.persist.mock.calls).toEqual([[true]])
  saved()
  await flushPromises()
  expect(mocks.persist.mock.calls).toEqual([[true], [false]])
  expect(showScrollbars.value).toBe(false)
})
