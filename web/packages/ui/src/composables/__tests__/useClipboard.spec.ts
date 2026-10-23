import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, effectScope } from 'vue'
import { installHiveUi } from '../../platform'
import { fakePlatform } from '../../test-utils/platform'
import { useClipboard } from '../useClipboard'

const mocks = { SetText: fakePlatform.clipboard.writeText }

// Run the composable inside an effect scope so onScopeDispose has a scope to
// register against (and so scope.stop() exercises the timer cleanup). The app
// context is what lets it inject the platform outside a component.
function inScope(resetDelay = 1000) {
  const app = createApp({})
  installHiveUi(app, fakePlatform)
  const scope = effectScope()
  const api = app.runWithContext(() => scope.run(() => useClipboard({ resetDelay })))!
  return { api, scope }
}

beforeEach(() => {
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
})

describe('useClipboard', () => {
  it('writes through the platform clipboard and flags success, then resets', async () => {
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

  it('flags error when the platform clipboard rejects', async () => {
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
