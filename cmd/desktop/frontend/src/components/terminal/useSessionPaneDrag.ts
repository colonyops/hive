import { ref, type Ref } from 'vue'
import { startDrag } from '@hive/ui/composables/useDragGesture'
import { resizeSplit, type LayoutBox, type SessionSplit } from '../../lib/sessionLayout'
import type { SessionPanes } from './useSessionPanes'

export interface TabDrop {
  pane: string
  gap: number
}

const DRAG_THRESHOLD_PX = 4
const KEY_STEP = 0.05

// A row starts flush with the sidebar, whose resize handle overlaps its first few
// pixels, so a drop at the very start of a row has to be read from the geometry.
const ROW_EDGE_PX = 12

function paneAt(x: number, y: number): string | undefined {
  const under = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-session-pane]')
  if (under?.dataset.sessionPane) return under.dataset.sessionPane
  for (const pane of document.querySelectorAll<HTMLElement>('section[data-session-pane]')) {
    const row = document.querySelector(`[data-testid="session-tab-row-${pane.dataset.sessionPane}"]`)
    const box = row?.getBoundingClientRect()
    if (box && y >= box.top && y <= box.bottom && x >= box.left - ROW_EDGE_PX && x <= box.right + ROW_EDGE_PX) {
      return pane.dataset.sessionPane
    }
  }
  return undefined
}

/** Where a pointer would drop a tab: the gap nearest it in the pane it is over. */
function dropAt(x: number, y: number): TabDrop | null {
  const id = paneAt(x, y)
  if (!id) return null
  const row = document.querySelector(`[data-testid="session-tab-row-${id}"]`)
  const tabs = [...(row?.querySelectorAll('[data-session-tab]') ?? [])]
  const gap = tabs.findIndex((tab) => {
    const box = tab.getBoundingClientRect()
    return x < box.left + box.width / 2
  })
  return { pane: id, gap: gap < 0 ? tabs.length : gap }
}

/**
 * Pointer drags rather than HTML drag and drop: WKWebView claims a native drag
 * for file drops, and a Terminal or a document iframe under the pointer would
 * swallow its events. Capture keeps them coming across both.
 */
export function useSessionPaneDrag(panes: Pick<SessionPanes, 'apply' | 'moveTabTo'>, host: Ref<HTMLElement | null>) {
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
  function startResize(event: PointerEvent, split: SessionSplit, box: LayoutBox): void {
    const rect = host.value?.getBoundingClientRect()
    if (!rect || event.button !== 0) return
    event.preventDefault()
    const ratioAt = (at: PointerEvent): number =>
      split.axis === 'row'
        ? (at.clientX - rect.left - box.x * rect.width) / (box.width * rect.width)
        : (at.clientY - rect.top - box.y * rect.height) / (box.height * rect.height)
    const offset = ratioAt(event) - split.ratio
    startDrag(event, {
      pointerCapture: true,
      onMove: (move) => panes.apply((current) => resizeSplit(current, split.id, ratioAt(move) - offset)),
    })
  }

  function stepResize(split: SessionSplit, direction: number): void {
    panes.apply((current) => resizeSplit(current, split.id, split.ratio + direction * KEY_STEP))
  }

  return { tabDrop, startTabDrag, startResize, stepResize }
}
