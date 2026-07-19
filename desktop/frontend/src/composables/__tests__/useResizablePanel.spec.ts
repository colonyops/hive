import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useResizablePanel } from '../useResizablePanel'

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() { return values.size },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value),
  }
}

beforeEach(() => {
  vi.stubGlobal('localStorage', memoryStorage())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function pointerEvent(type: string, clientX: number): PointerEvent {
  return new PointerEvent(type, { pointerId: 1, clientX, bubbles: true, cancelable: true })
}

/** Dispatches pointerdown on a real element (so `event.target` is set, the
 * way it would be for a real `@pointerdown="startResize"` handler) then
 * returns it for pointermove/pointerup dispatch on window, mirroring how the
 * composable actually tracks a drag. */
function beginDrag(startResize: (e: PointerEvent) => void, clientX: number): void {
  const handle = document.createElement('div')
  document.body.appendChild(handle)
  handle.addEventListener('pointerdown', (e) => startResize(e as PointerEvent))
  handle.dispatchEvent(pointerEvent('pointerdown', clientX))
}

describe('useResizablePanel', () => {
  it('initializes at defaultWidth when nothing is stored', () => {
    const { width } = useResizablePanel({ storageKey: 'hive.panel.a', defaultWidth: 250, min: 190, max: 480, edge: 'right' })
    expect(width.value).toBe(250)
  })

  it('restores a persisted width from localStorage', () => {
    localStorage.setItem('hive.panel.b', '300')
    const { width } = useResizablePanel({ storageKey: 'hive.panel.b', defaultWidth: 250, min: 190, max: 480, edge: 'right' })
    expect(width.value).toBe(300)
  })

  it('ignores an out-of-range stored value and falls back to default', () => {
    localStorage.setItem('hive.panel.c', '999')
    const { width } = useResizablePanel({ storageKey: 'hive.panel.c', defaultWidth: 250, min: 190, max: 480, edge: 'right' })
    expect(width.value).toBe(250)
  })

  it('ignores a non-numeric stored value and falls back to default', () => {
    localStorage.setItem('hive.panel.d', 'not-a-number')
    const { width } = useResizablePanel({ storageKey: 'hive.panel.d', defaultWidth: 250, min: 190, max: 480, edge: 'right' })
    expect(width.value).toBe(250)
  })

  it('a right-edge drag grows the panel as the pointer moves right, and persists only on pointerup', () => {
    const { width, startResize } = useResizablePanel({ storageKey: 'hive.panel.e', defaultWidth: 250, min: 190, max: 480, edge: 'right' })

    beginDrag(startResize, 100)
    window.dispatchEvent(pointerEvent('pointermove', 150))

    expect(width.value).toBe(300)
    expect(localStorage.getItem('hive.panel.e')).toBeNull() // not yet persisted mid-drag

    window.dispatchEvent(pointerEvent('pointerup', 150))
    expect(localStorage.getItem('hive.panel.e')).toBe('300')
  })

  it('a left-edge drag grows the panel as the pointer moves left', () => {
    const { width, startResize } = useResizablePanel({ storageKey: 'hive.panel.f', defaultWidth: 440, min: 360, max: 760, edge: 'left' })

    beginDrag(startResize, 500)
    window.dispatchEvent(pointerEvent('pointermove', 450)) // moved left by 50

    expect(width.value).toBe(490)

    window.dispatchEvent(pointerEvent('pointerup', 450))
    expect(localStorage.getItem('hive.panel.f')).toBe('490')
  })

  it('clamps the dragged width to [min, max]', () => {
    const { width, startResize } = useResizablePanel({ storageKey: 'hive.panel.g', defaultWidth: 250, min: 190, max: 480, edge: 'right' })

    beginDrag(startResize, 0)
    window.dispatchEvent(pointerEvent('pointermove', 10000))
    expect(width.value).toBe(480)

    window.dispatchEvent(pointerEvent('pointermove', -10000))
    expect(width.value).toBe(190)

    window.dispatchEvent(pointerEvent('pointerup', -10000))
    expect(localStorage.getItem('hive.panel.g')).toBe('190')
  })

  it('step() nudges and clamps the width and persists immediately', () => {
    const { width, step } = useResizablePanel({ storageKey: 'hive.panel.h', defaultWidth: 250, min: 190, max: 480, edge: 'right' })

    step(12)
    expect(width.value).toBe(262)
    expect(localStorage.getItem('hive.panel.h')).toBe('262')

    step(-1000)
    expect(width.value).toBe(190)
    expect(localStorage.getItem('hive.panel.h')).toBe('190')

    step(1000)
    expect(width.value).toBe(480)
  })

  it('stops tracking pointermove after pointerup', () => {
    const { width, startResize } = useResizablePanel({ storageKey: 'hive.panel.i', defaultWidth: 250, min: 190, max: 480, edge: 'right' })

    beginDrag(startResize, 0)
    window.dispatchEvent(pointerEvent('pointermove', 50))
    expect(width.value).toBe(300)

    window.dispatchEvent(pointerEvent('pointerup', 50))
    window.dispatchEvent(pointerEvent('pointermove', 200)) // should be ignored — drag already ended

    expect(width.value).toBe(300)
  })
})
