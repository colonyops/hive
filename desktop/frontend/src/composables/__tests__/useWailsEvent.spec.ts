import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'

const mocks = vi.hoisted(() => ({
  On: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
}))

import { useWailsEvent } from '../useWailsEvent'

describe('useWailsEvent', () => {
  beforeEach(() => vi.clearAllMocks())

  it('registers the handler and unsubscribes when its scope stops', () => {
    const unsubscribe = vi.fn()
    const handler = vi.fn()
    mocks.On.mockReturnValue(unsubscribe)
    const scope = effectScope()

    scope.run(() => useWailsEvent('test:event', handler))

    expect(mocks.On).toHaveBeenCalledWith('test:event', handler)
    scope.stop()
    expect(unsubscribe).toHaveBeenCalledOnce()
  })
})
