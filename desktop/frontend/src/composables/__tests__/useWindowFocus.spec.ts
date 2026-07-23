import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  Focused: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/windowservice', () => ({
  Focused: mocks.Focused,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

async function loadComposable() {
  const { useWindowFocus } = await import('../useWindowFocus')
  return useWindowFocus()
}

describe('useWindowFocus', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.Focused.mockResolvedValue(false)
  })

  it('seeds the optimistic focus state from the native service', async () => {
    let resolveFocused!: (focused: boolean) => void
    mocks.Focused.mockImplementation(() => new Promise<boolean>((resolve) => { resolveFocused = resolve }))
    const windowFocus = await loadComposable()

    expect(windowFocus.focused.value).toBe(true)
    resolveFocused(false)
    await flushPromises()
    expect(windowFocus.focused.value).toBe(false)
    expect(mocks.On.mock.calls.map(([name]) => name)).toEqual(['window:focus', 'window:blur'])
  })

  it('updates focus state from native focus events', async () => {
    const windowFocus = await loadComposable()
    await flushPromises()
    expect(windowFocus.focused.value).toBe(false)

    mocks.On.mock.calls[0][1]()
    expect(windowFocus.focused.value).toBe(true)
    mocks.On.mock.calls[1][1]()
    expect(windowFocus.focused.value).toBe(false)
  })

  it('does not let an older seed overwrite a newer focus event', async () => {
    let resolveFocused!: (focused: boolean) => void
    mocks.Focused.mockImplementation(() => new Promise<boolean>((resolve) => { resolveFocused = resolve }))
    const windowFocus = await loadComposable()

    mocks.On.mock.calls[1][1]()
    expect(windowFocus.focused.value).toBe(false)
    resolveFocused(true)
    await flushPromises()

    expect(windowFocus.focused.value).toBe(false)
  })
})
