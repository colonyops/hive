import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref, type EffectScope } from 'vue'
import { useEscapeToClose } from '../useEscapeToClose'

function pressEscape(): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
}

const scopes: EffectScope[] = []

function register(onEscape: () => void, options?: Parameters<typeof useEscapeToClose>[1]): EffectScope {
  const scope = effectScope()
  scope.run(() => useEscapeToClose(onEscape, options))
  scopes.push(scope)
  return scope
}

afterEach(() => {
  scopes.splice(0).forEach((scope) => scope.stop())
})

describe('useEscapeToClose', () => {
  it('calls the callback for Escape and removes its listener when the scope stops', () => {
    const onEscape = vi.fn()
    const scope = register(onEscape)

    pressEscape()
    expect(onEscape).toHaveBeenCalledOnce()

    scope.stop()
    pressEscape()
    expect(onEscape).toHaveBeenCalledOnce()
  })

  it('respects a reactive enabled guard', async () => {
    const onEscape = vi.fn()
    const enabled = ref(false)
    register(onEscape, { enabled })

    pressEscape()
    expect(onEscape).not.toHaveBeenCalled()

    enabled.value = true
    await nextTick()
    pressEscape()
    expect(onEscape).toHaveBeenCalledOnce()
  })

  it('fires only the most recently registered layer', () => {
    const settings = vi.fn()
    const drawer = vi.fn()
    register(settings)
    const drawerScope = register(drawer)

    pressEscape()
    expect(drawer).toHaveBeenCalledOnce()
    expect(settings).not.toHaveBeenCalled()

    drawerScope.stop()
    pressEscape()
    expect(settings).toHaveBeenCalledOnce()
  })

  it('skips a disabled layer for the one beneath it', () => {
    const owner = vi.fn()
    const sheet = vi.fn()
    register(owner)
    register(sheet, { enabled: false })

    pressEscape()
    expect(owner).toHaveBeenCalledOnce()
    expect(sheet).not.toHaveBeenCalled()
  })

  it('moves a layer to the top when it becomes enabled', async () => {
    const paletteOpen = ref(false)
    const palette = vi.fn()
    const tasks = vi.fn()
    register(palette, { enabled: paletteOpen })
    register(tasks)

    paletteOpen.value = true
    await nextTick()
    pressEscape()
    expect(palette).toHaveBeenCalledOnce()
    expect(tasks).not.toHaveBeenCalled()
  })
})
