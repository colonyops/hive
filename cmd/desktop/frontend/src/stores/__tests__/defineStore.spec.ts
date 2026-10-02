import { describe, expect, it, vi } from 'vitest'
import { effectScope, onScopeDispose, ref } from 'vue'
import { defineStore, resetStores } from '../defineStore'

describe('defineStore', () => {
  it('runs setup once and hands every caller the same instance', () => {
    const setup = vi.fn(() => ({ count: ref(0) }))
    const useCounter = defineStore('counter', setup)

    const first = useCounter()
    const second = useCounter()

    expect(second).toBe(first)
    expect(setup).toHaveBeenCalledOnce()
  })

  it('owns the effects its setup creates, not the scope that first used it', () => {
    const dispose = vi.fn()
    const useStore = defineStore('owned', () => {
      onScopeDispose(dispose)
      return { ok: true }
    })
    const caller = effectScope()

    caller.run(() => useStore())
    caller.stop()
    expect(dispose).not.toHaveBeenCalled()

    resetStores()
    expect(dispose).toHaveBeenCalledOnce()
  })

  it('runs setup again after resetStores', () => {
    const useCounter = defineStore('counter-reset', () => ({ count: ref(0) }))
    const stale = useCounter()
    stale.count.value = 3

    resetStores()
    const fresh = useCounter()

    expect(fresh).not.toBe(stale)
    expect(fresh.count.value).toBe(0)
  })

  it('resets a store another store used during setup', () => {
    const disposeInner = vi.fn()
    const useInner = defineStore('inner', () => {
      onScopeDispose(disposeInner)
      return { value: ref('inner') }
    })
    const useOuter = defineStore('outer', () => ({ inner: useInner().value }))

    expect(useOuter().inner.value).toBe('inner')
    resetStores()
    expect(disposeInner).toHaveBeenCalledOnce()
  })

  it('forgets every store even when one fails to stop, then reports the failure', () => {
    const useBroken = defineStore('broken', () => {
      onScopeDispose(() => {
        throw new Error('cleanup failed')
      })
      return { ok: true }
    })
    const disposeHealthy = vi.fn()
    const useHealthy = defineStore('healthy', () => {
      onScopeDispose(disposeHealthy)
      return { ok: true }
    })
    useBroken()
    const healthy = useHealthy()

    expect(() => resetStores()).toThrow('cleanup failed')
    expect(disposeHealthy).toHaveBeenCalledOnce()
    expect(() => resetStores()).not.toThrow()
    expect(useHealthy()).not.toBe(healthy)
  })

  it('refuses a store that uses itself during setup', () => {
    const useLoop = defineStore('loop', (): { ok: boolean } => useLoop())

    expect(() => useLoop()).toThrow('store "loop" uses itself during setup')
  })

  it('keeps no instance from a setup that threw and tries again next time', () => {
    const dispose = vi.fn()
    let attempts = 0
    const useFlaky = defineStore('flaky', () => {
      onScopeDispose(dispose)
      attempts++
      if (attempts === 1) throw new Error('first time fails')
      return { attempts }
    })

    expect(() => useFlaky()).toThrow('first time fails')
    expect(dispose).toHaveBeenCalledOnce()
    expect(useFlaky().attempts).toBe(2)
  })
})
