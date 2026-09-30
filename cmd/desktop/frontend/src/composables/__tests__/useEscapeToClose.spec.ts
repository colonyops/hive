import { describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { useEscapeToClose } from '../useEscapeToClose'

function pressEscape(): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
}

describe('useEscapeToClose', () => {
  it('calls the callback for Escape and removes its listener when the scope stops', () => {
    const onEscape = vi.fn()
    const scope = effectScope()

    scope.run(() => useEscapeToClose(onEscape))
    pressEscape()
    expect(onEscape).toHaveBeenCalledOnce()

    scope.stop()
    pressEscape()
    expect(onEscape).toHaveBeenCalledOnce()
  })

  it('respects a reactive enabled guard', () => {
    const onEscape = vi.fn()
    const enabled = ref(false)
    const scope = effectScope()

    scope.run(() => useEscapeToClose(onEscape, { enabled }))
    pressEscape()
    expect(onEscape).not.toHaveBeenCalled()

    enabled.value = true
    pressEscape()
    expect(onEscape).toHaveBeenCalledOnce()
    scope.stop()
  })
})
