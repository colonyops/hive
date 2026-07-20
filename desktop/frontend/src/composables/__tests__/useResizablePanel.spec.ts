import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { useResizablePanel } from '../useResizablePanel'

// Size persists to localStorage (via VueUse useStorage); isolate each test.
beforeEach(() => localStorage.clear())

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
  it('initializes at defaultSize when nothing is stored', () => {
    const { size } = useResizablePanel({ storageKey: 'hive.panel.a', defaultSize: 250, min: 190, max: 480, edge: 'right' })
    expect(size.value).toBe(250)
  })

  it('restores a persisted width from localStorage', () => {
    localStorage.setItem('hive.panel.b', '300')
    const { size } = useResizablePanel({ storageKey: 'hive.panel.b', defaultSize: 250, min: 190, max: 480, edge: 'right' })
    expect(size.value).toBe(300)
  })

  it('ignores an out-of-range stored value and falls back to default', () => {
    localStorage.setItem('hive.panel.c', '999')
    const { size } = useResizablePanel({ storageKey: 'hive.panel.c', defaultSize: 250, min: 190, max: 480, edge: 'right' })
    expect(size.value).toBe(250)
  })

  it('ignores a non-numeric stored value and falls back to default', () => {
    localStorage.setItem('hive.panel.d', 'not-a-number')
    const { size } = useResizablePanel({ storageKey: 'hive.panel.d', defaultSize: 250, min: 190, max: 480, edge: 'right' })
    expect(size.value).toBe(250)
  })

  it('a right-edge drag grows the panel as the pointer moves right, and persists', async () => {
    const { size, startResize } = useResizablePanel({ storageKey: 'hive.panel.e', defaultSize: 250, min: 190, max: 480, edge: 'right' })

    beginDrag(startResize, 100)
    window.dispatchEvent(pointerEvent('pointermove', 150))
    window.dispatchEvent(pointerEvent('pointerup', 150))

    expect(size.value).toBe(300)
    await nextTick()
    expect(localStorage.getItem('hive.panel.e')).toBe('300')
  })

  it('a left-edge drag grows the panel as the pointer moves left', async () => {
    const { size, startResize } = useResizablePanel({ storageKey: 'hive.panel.f', defaultSize: 440, min: 360, max: 760, edge: 'left' })

    beginDrag(startResize, 500)
    window.dispatchEvent(pointerEvent('pointermove', 450)) // moved left by 50
    window.dispatchEvent(pointerEvent('pointerup', 450))

    expect(size.value).toBe(490)
    await nextTick()
    expect(localStorage.getItem('hive.panel.f')).toBe('490')
  })

  it('clamps the dragged width to [min, max]', async () => {
    const { size, startResize } = useResizablePanel({ storageKey: 'hive.panel.g', defaultSize: 250, min: 190, max: 480, edge: 'right' })

    beginDrag(startResize, 0)
    window.dispatchEvent(pointerEvent('pointermove', 10000))
    expect(size.value).toBe(480)

    window.dispatchEvent(pointerEvent('pointermove', -10000))
    expect(size.value).toBe(190)

    window.dispatchEvent(pointerEvent('pointerup', -10000))
    await nextTick()
    expect(localStorage.getItem('hive.panel.g')).toBe('190')
  })

  it('step() nudges and clamps the width and persists', async () => {
    const { size, step } = useResizablePanel({ storageKey: 'hive.panel.h', defaultSize: 250, min: 190, max: 480, edge: 'right' })

    step(12)
    expect(size.value).toBe(262)
    await nextTick()
    expect(localStorage.getItem('hive.panel.h')).toBe('262')

    step(-1000)
    expect(size.value).toBe(190)
    await nextTick()
    expect(localStorage.getItem('hive.panel.h')).toBe('190')

    step(1000)
    expect(size.value).toBe(480)
  })

  it('a bottom-edge drag grows the panel as the pointer moves down (vertical axis)', async () => {
    const { size, startResize } = useResizablePanel({ storageKey: 'hive.panel.v', defaultSize: 240, min: 96, max: 640, edge: 'bottom' })

    const handle = document.createElement('div')
    document.body.appendChild(handle)
    handle.addEventListener('pointerdown', (e) => startResize(e as PointerEvent))
    handle.dispatchEvent(new PointerEvent('pointerdown', { pointerId: 1, clientY: 100, bubbles: true, cancelable: true }))
    window.dispatchEvent(new PointerEvent('pointermove', { pointerId: 1, clientY: 160 })) // moved down by 60
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, clientY: 160 }))

    expect(size.value).toBe(300)
    await nextTick()
    expect(localStorage.getItem('hive.panel.v')).toBe('300')
  })

  it('stops tracking pointermove after pointerup', () => {
    const { size, startResize } = useResizablePanel({ storageKey: 'hive.panel.i', defaultSize: 250, min: 190, max: 480, edge: 'right' })

    beginDrag(startResize, 0)
    window.dispatchEvent(pointerEvent('pointermove', 50))
    expect(size.value).toBe(300)

    window.dispatchEvent(pointerEvent('pointerup', 50))
    window.dispatchEvent(pointerEvent('pointermove', 200)) // should be ignored — drag already ended

    expect(size.value).toBe(300)
  })
})
