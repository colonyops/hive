import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'

const mocks = vi.hoisted(() => ({ SetText: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: mocks.SetText } }))

import { useClipboard } from '../useClipboard'

// Run the composable inside an effect scope so onScopeDispose has a scope to
// register against (and so scope.stop() exercises the timer cleanup).
function inScope(resetDelay = 1000) {
  const scope = effectScope()
  const api = scope.run(() => useClipboard({ resetDelay }))!
  return { api, scope }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
})

describe('useClipboard', () => {
  it('writes through the native Wails clipboard and flags success, then resets', async () => {
    mocks.SetText.mockResolvedValue(undefined)
    const { api, scope } = inScope()

    await api.copy('hello')
    expect(mocks.SetText).toHaveBeenCalledWith('hello')
    expect(api.status.value).toBe('success')
    expect(api.copied.value).toBe(true)

    vi.advanceTimersByTime(1000)
    expect(api.status.value).toBe('idle')
    expect(api.copied.value).toBe(false)

    scope.stop()
  })

  it('flags error when the native clipboard rejects', async () => {
    mocks.SetText.mockRejectedValue(new Error('no clipboard'))
    const { api, scope } = inScope()

    await api.copy('x')
    expect(api.status.value).toBe('error')
    expect(api.copied.value).toBe(false)

    scope.stop()
  })

  it('stops the reset timer when the scope is disposed', async () => {
    mocks.SetText.mockResolvedValue(undefined)
    const { api, scope } = inScope()

    await api.copy('x')
    expect(api.status.value).toBe('success')

    scope.stop()
    // Timer was cleared on dispose, so advancing does not mutate a disposed scope.
    vi.advanceTimersByTime(1000)
    expect(api.status.value).toBe('success')
  })
})
