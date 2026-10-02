import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useWindowFocus } from '../useWindowFocus'

const mocks = vi.hoisted(() => ({
  Focused: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: mocks.Focused,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function handler(name: string): () => void {
  return mocks.On.mock.calls.find(([event]) => event === name)?.[1] as () => void
}

describe('useWindowFocus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.Focused.mockResolvedValue(false)
  })

  it('seeds the optimistic focus state from the native service', async () => {
    let resolveFocused!: (focused: boolean) => void
    mocks.Focused.mockImplementation(
      () =>
        new Promise<boolean>((resolve) => {
          resolveFocused = resolve
        }),
    )
    const windowFocus = useWindowFocus()

    expect(windowFocus.focused.value).toBe(true)
    resolveFocused(false)
    await flushPromises()
    expect(windowFocus.focused.value).toBe(false)
    expect(mocks.On.mock.calls.map(([name]) => name as string)).toEqual(['window:focus', 'window:blur'])
  })

  it('updates focus state from native focus events', async () => {
    const windowFocus = useWindowFocus()
    await flushPromises()
    expect(windowFocus.focused.value).toBe(false)

    handler('window:focus')()
    expect(windowFocus.focused.value).toBe(true)
    handler('window:blur')()
    expect(windowFocus.focused.value).toBe(false)
  })

  it('does not let an older seed overwrite a newer focus event', async () => {
    let resolveFocused!: (focused: boolean) => void
    mocks.Focused.mockImplementation(
      () =>
        new Promise<boolean>((resolve) => {
          resolveFocused = resolve
        }),
    )
    const windowFocus = useWindowFocus()

    handler('window:blur')()
    expect(windowFocus.focused.value).toBe(false)
    resolveFocused(true)
    await flushPromises()

    expect(windowFocus.focused.value).toBe(false)
  })
})
