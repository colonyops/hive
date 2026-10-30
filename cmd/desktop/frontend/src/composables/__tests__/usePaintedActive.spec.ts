import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { usePaintedActive } from '../usePaintedActive'

function setup(initial: boolean, options?: { preloaded?: boolean }) {
  const active = ref(initial)
  const scope = effectScope()
  const painted = scope.run(() => usePaintedActive(() => active.value, options))!
  return { active, painted, stop: () => scope.stop() }
}

describe('usePaintedActive', () => {
  beforeEach(() => {
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'requestAnimationFrame', 'cancelAnimationFrame', 'Date', 'performance'],
    })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('answers at once on a first load', () => {
    expect(setup(true).painted.value).toBe(true)
    expect(setup(false).painted.value).toBe(false)
  })

  it('treats a mount with preloaded data as a reveal', async () => {
    const { painted } = setup(true, { preloaded: true })
    expect(painted.value).toBe(false)
    vi.advanceTimersToNextFrame()
    vi.advanceTimersToNextFrame()
    await vi.advanceTimersByTimeAsync(50)
    expect(painted.value).toBe(true)
  })

  it('turns true only after the reveal has painted', async () => {
    const { active, painted } = setup(false)
    active.value = true
    await nextTick()
    expect(painted.value).toBe(false)
    await vi.advanceTimersByTimeAsync(16)
    expect(painted.value).toBe(false)
    await vi.advanceTimersByTimeAsync(100)
    expect(painted.value).toBe(true)
  })

  it('goes false as soon as the mode hides', async () => {
    const { active, painted } = setup(true)
    active.value = false
    await nextTick()
    expect(painted.value).toBe(false)
  })

  it('drops a reveal that was left before it painted', async () => {
    const { active, painted } = setup(false)
    active.value = true
    await nextTick()
    active.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(500)
    expect(painted.value).toBe(false)
  })

  it('still answers when no frames arrive, as in a covered window', async () => {
    const { active, painted } = setup(false)
    vi.stubGlobal('requestAnimationFrame', () => 0)
    active.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)
    expect(painted.value).toBe(true)
    vi.unstubAllGlobals()
  })
})
