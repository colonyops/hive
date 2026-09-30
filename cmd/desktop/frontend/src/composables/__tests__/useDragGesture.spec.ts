import { describe, expect, it, vi } from 'vitest'
import { startDrag } from '../useDragGesture'

function pointerEvent(type: string, clientX: number): PointerEvent {
  return new PointerEvent(type, { pointerId: 7, clientX, bubbles: true, cancelable: true })
}

describe('startDrag', () => {
  it('tracks moves through pointerup, then removes its listeners and releases pointer capture', () => {
    const target = document.createElement('div')
    const onMove = vi.fn()
    const onEnd = vi.fn()
    const setPointerCapture = vi.fn()
    const releasePointerCapture = vi.fn()
    Object.defineProperties(target, {
      setPointerCapture: { value: setPointerCapture },
      releasePointerCapture: { value: releasePointerCapture },
    })
    target.addEventListener('pointerdown', (event) => {
      startDrag(event as PointerEvent, { onMove, onEnd, pointerCapture: true })
    })

    target.dispatchEvent(pointerEvent('pointerdown', 10))
    expect(setPointerCapture).toHaveBeenCalledWith(7)

    const move = pointerEvent('pointermove', 20)
    window.dispatchEvent(move)
    expect(onMove).toHaveBeenCalledWith(move)

    const up = pointerEvent('pointerup', 20)
    window.dispatchEvent(up)
    expect(onEnd).toHaveBeenCalledWith(up)
    expect(releasePointerCapture).toHaveBeenCalledWith(7)

    window.dispatchEvent(pointerEvent('pointermove', 30))
    expect(onMove).toHaveBeenCalledTimes(1)
  })

  it('returns a stop function for ending a drag before pointerup', () => {
    const onMove = vi.fn()
    const stop = startDrag(pointerEvent('pointerdown', 0), { onMove })

    stop()
    window.dispatchEvent(pointerEvent('pointermove', 10))

    expect(onMove).not.toHaveBeenCalled()
  })
})
