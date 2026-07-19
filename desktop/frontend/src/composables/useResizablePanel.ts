import { ref, type Ref } from 'vue'

/** Which side of the panel the drag handle sits on — determines the sign of
 * the drag delta. A handle on the LEFT edge sits on a right-docked panel
 * (e.g. a drawer pinned to the window's right edge): dragging the pointer
 * left grows the panel. A handle on the RIGHT edge sits on a left-docked
 * panel (e.g. the sidebar): dragging the pointer right grows it. */
export type ResizeEdge = 'left' | 'right'

export interface UseResizablePanelOptions {
  /** localStorage key the width is persisted under, e.g. "hive.panel.sidebar". */
  storageKey: string
  defaultWidth: number
  min: number
  max: number
  edge: ResizeEdge
}

export interface UseResizablePanelReturn {
  /** Current panel width in px — bind via `:style="{ width: width + 'px' }"`. */
  width: Ref<number>
  /** Pointerdown handler for the drag handle — starts tracking the drag. */
  startResize: (event: PointerEvent) => void
  /** Nudges width by `deltaPx` (positive or negative), clamped, and persists — for keyboard resize. */
  step: (deltaPx: number) => void
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export function useResizablePanel(options: UseResizablePanelOptions): UseResizablePanelReturn {
  const { storageKey, defaultWidth, min, max, edge } = options

  function readStored(): number {
    const raw = localStorage.getItem(storageKey)
    if (raw === null) return defaultWidth
    const parsed = Number(raw)
    if (!Number.isFinite(parsed) || parsed < min || parsed > max) return defaultWidth
    return parsed
  }

  const width = ref(readStored())

  // See ResizeEdge above: 'left' inverts the delta so dragging toward the
  // panel's interior (pointer moving right) shrinks it.
  const sign = edge === 'left' ? -1 : 1

  function persist(): void {
    localStorage.setItem(storageKey, String(width.value))
  }

  function startResize(event: PointerEvent): void {
    const target = event.target as Element | null
    const pointerId = event.pointerId
    target?.setPointerCapture?.(pointerId)

    const startX = event.clientX
    const startWidth = width.value

    function onMove(e: PointerEvent): void {
      width.value = clamp(startWidth + sign * (e.clientX - startX), min, max)
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
    width.value = clamp(width.value + deltaPx, min, max)
    persist()
  }

  return { width, startResize, step }
}
