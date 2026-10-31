import { ref, type Ref } from 'vue'
import { startDrag } from '@hive/ui/composables/useDragGesture'
import { resizeSplit } from '../../lib/sessionLayout'
import type { SessionPanes } from './useSessionPanes'

export interface TabDrop {
  pane: number
  gap: number
}

const DRAG_THRESHOLD_PX = 4
const KEY_STEP = 0.05

/** Where a pointer would drop a tab: the gap nearest it in the pane it is over. */
function dropAt(x: number, y: number): TabDrop | null {
  const pane = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-session-pane]')
  if (!pane) return null
  const index = Number(pane.dataset.sessionPane)
  const row = document.querySelector(`[data-testid="session-tab-row-${index}"]`)
  const tabs = [...(row?.querySelectorAll('[data-session-tab]') ?? [])]
  const gap = tabs.findIndex((tab) => {
    const box = tab.getBoundingClientRect()
    return x < box.left + box.width / 2
  })
  return { pane: index, gap: gap < 0 ? tabs.length : gap }
}

/**
 * Pointer drags rather than HTML drag and drop: WKWebView claims a native drag
 * for file drops, and a Terminal or a document iframe under the pointer would
 * swallow its events. Capture keeps them coming across both.
 */
export function useSessionPaneDrag(
  panes: Pick<SessionPanes, 'apply' | 'moveTabTo'>,
  host: Ref<HTMLElement | null>,
  currentRatio: () => number,
) {
  const tabDrop = ref<TabDrop | null>(null)

  function startTabDrag(event: PointerEvent, tabId: string): void {
    if (event.button !== 0) return
    // Unclaimed, WebKit can read the press as the start of its own selection or
    // drag and cancel the pointer. The click that selects the tab still fires.
    event.preventDefault()
    const { clientX: startX, clientY: startY } = event
    let dragging = false
    startDrag(event, {
      pointerCapture: true,
      onMove: (move) => {
        dragging ||= Math.hypot(move.clientX - startX, move.clientY - startY) >= DRAG_THRESHOLD_PX
        if (dragging) tabDrop.value = dropAt(move.clientX, move.clientY)
      },
      onEnd: () => {
        const drop = tabDrop.value
        tabDrop.value = null
        if (drop) panes.moveTabTo(tabId, drop.pane, drop.gap)
      },
      onCancel: () => (tabDrop.value = null),
    })
  }

  // Keeps the offset from the boundary to where it was grabbed, because a stacked
  // tab row is a grab area 30px deep and the boundary must not jump to the pointer.
  function startResize(event: PointerEvent, stacked: boolean): void {
    const box = host.value?.getBoundingClientRect()
    if (!box || event.button !== 0) return
    event.preventDefault()
    const ratioAt = (at: PointerEvent): number =>
      stacked ? (at.clientY - box.top) / box.height : (at.clientX - box.left) / box.width
    const offset = ratioAt(event) - currentRatio()
    startDrag(event, {
      pointerCapture: true,
      onMove: (move) => panes.apply((current) => resizeSplit(current, ratioAt(move) - offset)),
    })
  }

  function stepResize(direction: number): void {
    panes.apply((current) => resizeSplit(current, current.splitRatio + direction * KEY_STEP))
  }

  return { tabDrop, startTabDrag, startResize, stepResize }
}
