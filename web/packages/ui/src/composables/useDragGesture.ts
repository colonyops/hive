export interface DragGestureOptions {
  onMove: (event: PointerEvent) => void
  onEnd?: (event: PointerEvent) => void
  /** The system took the pointer away, so the drag ends where it is and commits nothing. */
  onCancel?: () => void
  pointerCapture?: boolean
}

// The browser starts a text selection from the press whatever is under it, and
// pointer capture does not stop that, so a drag would highlight the text it crosses.
function preventSelection(event: Event): void {
  event.preventDefault()
}

/** Starts a window-tracked pointer drag and returns a function that stops it early. */
export function startDrag(event: PointerEvent, options: DragGestureOptions): () => void {
  const target = options.pointerCapture ? (event.target as Element | null) : null
  const pointerId = event.pointerId
  target?.setPointerCapture?.(pointerId)

  let stopped = false

  function stop(): void {
    if (stopped) return
    stopped = true
    window.removeEventListener('pointermove', options.onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onCancel)
    window.removeEventListener('selectstart', preventSelection, true)
    target?.releasePointerCapture?.(pointerId)
  }

  function onUp(upEvent: PointerEvent): void {
    stop()
    options.onEnd?.(upEvent)
  }

  function onCancel(): void {
    stop()
    options.onCancel?.()
  }

  window.addEventListener('pointermove', options.onMove)
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onCancel)
  window.addEventListener('selectstart', preventSelection, true)

  return stop
}
