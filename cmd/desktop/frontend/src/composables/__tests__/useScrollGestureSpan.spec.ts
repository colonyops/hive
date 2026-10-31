import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref, type EffectScope } from 'vue'
import { resetFrameCollectorForTests } from '../../lib/frameCollector'
import { useScrollGestureSpan, type ScrollGestureAttrs } from '../useScrollGestureSpan'

describe('useScrollGestureSpan', () => {
  let scope: EffectScope
  let el: HTMLElement
  const record = vi.fn<(durationMs: number, attrs: ScrollGestureAttrs) => void>()

  beforeEach(async () => {
    vi.useFakeTimers()
    record.mockReset()
    el = document.createElement('div')
    scope = effectScope()
    scope.run(() => useScrollGestureSpan(ref(el), record))
    await nextTick()
  })

  afterEach(() => {
    scope.stop()
    resetFrameCollectorForTests()
    vi.useRealTimers()
  })

  function scrollTo(top: number): void {
    el.scrollTop = top
    el.dispatchEvent(new Event('scroll'))
  }

  it('reports one span for a wheel gesture once it goes idle', () => {
    el.dispatchEvent(new Event('wheel'))
    vi.advanceTimersByTime(10)
    scrollTo(30)
    vi.advanceTimersByTime(40)
    el.dispatchEvent(new Event('wheel'))
    scrollTo(10)
    expect(record).not.toHaveBeenCalled()

    vi.advanceTimersByTime(150)

    expect(record).toHaveBeenCalledOnce()
    const [durationMs, attrs] = record.mock.calls[0]
    expect(durationMs).toBe(50)
    expect(attrs).toMatchObject({ wheelEvents: 2, scrollEvents: 2, pixels: 50, scrollGapWorst: 40 })
  })

  it('ignores a scroll that no wheel started', () => {
    scrollTo(100)
    vi.advanceTimersByTime(500)
    expect(record).not.toHaveBeenCalled()
  })

  it('reports a gesture still open when the scope ends', () => {
    el.dispatchEvent(new Event('wheel'))
    scope.stop()
    expect(record).toHaveBeenCalledOnce()
  })
})
