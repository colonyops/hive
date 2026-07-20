import { ref, type Ref } from 'vue'

/** Which edge of the panel the drag handle sits on. This picks both the drag
 * axis and the sign of the delta:
 *   - 'left'/'right' resize WIDTH (horizontal drag).
 *   - 'top'/'bottom' resize HEIGHT (vertical drag).
 * A handle on the LEFT edge sits on a right-docked panel (drag left grows it);
 * on the RIGHT edge, a left-docked panel (drag right grows it). Likewise a
 * BOTTOM handle sits on a top-docked panel (drag down grows it), and a TOP
 * handle on a bottom-docked panel (drag up grows it). */
export type ResizeEdge = 'left' | 'right' | 'top' | 'bottom'

export interface UseResizablePanelOptions {
  /** localStorage key the size is persisted under, e.g. "hive.panel.sidebar". */
  storageKey: string
  /** Starting size in px along the resize axis (width for left/right, height for top/bottom). */
  defaultSize: number
  min: number
  max: number
  edge: ResizeEdge
}

export interface UseResizablePanelReturn {
  /** Current panel size in px along the resize axis — bind to width or height. */
  size: Ref<number>
  /** Pointerdown handler for the drag handle — starts tracking the drag. */
  startResize: (event: PointerEvent) => void
  /** Nudges size by `deltaPx` (positive or negative), clamped, and persists — for keyboard resize. */
  step: (deltaPx: number) => void
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export function useResizablePanel(options: UseResizablePanelOptions): UseResizablePanelReturn {
  const { storageKey, defaultSize, min, max, edge } = options

  function readStored(): number {
    const raw = localStorage.getItem(storageKey)
    if (raw === null) return defaultSize
    const parsed = Number(raw)
    if (!Number.isFinite(parsed) || parsed < min || parsed > max) return defaultSize
    return parsed
  }

  const size = ref(readStored())

  const vertical = edge === 'top' || edge === 'bottom'
  // See ResizeEdge above: the 'left'/'top' edges invert the delta so dragging
  // toward the panel's interior shrinks it.
  const sign = edge === 'left' || edge === 'top' ? -1 : 1

  function persist(): void {
    localStorage.setItem(storageKey, String(size.value))
  }

  function startResize(event: PointerEvent): void {
    const target = event.target as Element | null
    const pointerId = event.pointerId
    target?.setPointerCapture?.(pointerId)

    const start = vertical ? event.clientY : event.clientX
    const startSize = size.value

    function onMove(e: PointerEvent): void {
      const pos = vertical ? e.clientY : e.clientX
      size.value = clamp(startSize + sign * (pos - start), min, max)
    }

    function onUp(): void {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      target?.releasePointerCapture?.(pointerId)
      persist()
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  function step(deltaPx: number): void {
    size.value = clamp(size.value + deltaPx, min, max)
    persist()
  }

  return { size, startResize, step }
}
