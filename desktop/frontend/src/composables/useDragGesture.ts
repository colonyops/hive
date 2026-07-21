export interface DragGestureOptions {
  onMove: (event: PointerEvent) => void
  onEnd?: (event: PointerEvent) => void
  pointerCapture?: boolean
}

/** Starts a window-tracked pointer drag and returns a function that stops it early. */
export function startDrag(event: PointerEvent, options: DragGestureOptions): () => void {
  const target = options.pointerCapture ? event.target as Element | null : null
  const pointerId = event.pointerId
  target?.setPointerCapture?.(pointerId)

  let stopped = false

  function stop(): void {
    if (stopped) return
    stopped = true
    window.removeEventListener('pointermove', options.onMove)
    window.removeEventListener('pointerup', onUp)
    target?.releasePointerCapture?.(pointerId)
  }

  function onUp(upEvent: PointerEvent): void {
    stop()
    options.onEnd?.(upEvent)
  }

  window.addEventListener('pointermove', options.onMove)
  window.addEventListener('pointerup', onUp)

  return stop
}
