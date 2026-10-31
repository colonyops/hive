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

  it('reports a cancelled drag through onCancel, not onEnd', () => {
    const onEnd = vi.fn()
    const onCancel = vi.fn()
    startDrag(pointerEvent('pointerdown', 0), { onMove: vi.fn(), onEnd, onCancel })

    window.dispatchEvent(pointerEvent('pointercancel', 0))

    expect(onCancel).toHaveBeenCalledOnce()
    expect(onEnd).not.toHaveBeenCalled()
  })

  it('blocks text selection only for the length of the drag', () => {
    const selectStart = () => {
      const event = new Event('selectstart', { bubbles: true, cancelable: true })
      document.body.dispatchEvent(event)
      return event.defaultPrevented
    }

    expect(selectStart()).toBe(false)
    startDrag(pointerEvent('pointerdown', 0), { onMove: vi.fn() })
    expect(selectStart()).toBe(true)

    window.dispatchEvent(pointerEvent('pointerup', 0))
    expect(selectStart()).toBe(false)
  })

  it('ends the drag when the browser cancels the pointer', () => {
    const onMove = vi.fn()
    startDrag(pointerEvent('pointerdown', 0), { onMove })

    window.dispatchEvent(pointerEvent('pointercancel', 0))
    window.dispatchEvent(pointerEvent('pointermove', 10))

    expect(onMove).not.toHaveBeenCalled()
    const event = new Event('selectstart', { bubbles: true, cancelable: true })
    document.body.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(false)
  })
})
