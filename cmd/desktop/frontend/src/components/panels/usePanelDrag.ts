import { ref, type Ref } from 'vue'
import { startDrag } from '@hive/ui/composables/useDragGesture'
import { resizeSplit, type PanelBox, type PanelLayout, type PanelSplit } from '../../lib/panelLayout'

export interface TabDrop {
  panel: string
  gap: number
}

/** What a drag changes: the layout, through its owner, and a tab's place. */
export interface PanelDragTarget {
  apply(rule: (layout: PanelLayout) => PanelLayout): void
  moveTabTo(tabId: string, panel: string, gap: number): void
}

const DRAG_THRESHOLD_PX = 4
const KEY_STEP = 0.05

// A row starts flush with the sidebar, whose resize handle overlaps its first few
// pixels, so a drop at the very start of a row has to be read from the geometry.
const ROW_EDGE_PX = 12

function panelAt(x: number, y: number): string | undefined {
  const under = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-panel]')
  if (under?.dataset.panel) return under.dataset.panel
  for (const panel of document.querySelectorAll<HTMLElement>('section[data-panel]')) {
    const row = document.querySelector(`[data-testid="panel-tab-row-${panel.dataset.panel}"]`)
    const box = row?.getBoundingClientRect()
    if (box && y >= box.top && y <= box.bottom && x >= box.left - ROW_EDGE_PX && x <= box.right + ROW_EDGE_PX) {
      return panel.dataset.panel
    }
  }
  return undefined
}

/** Where a pointer would drop a tab: the gap nearest it in the panel it is over. */
function dropAt(x: number, y: number): TabDrop | null {
  const id = panelAt(x, y)
  if (!id) return null
  const row = document.querySelector(`[data-testid="panel-tab-row-${id}"]`)
  const tabs = [...(row?.querySelectorAll('[data-panel-tab]') ?? [])]
  const gap = tabs.findIndex((tab) => {
    const box = tab.getBoundingClientRect()
    return x < box.left + box.width / 2
  })
  return { panel: id, gap: gap < 0 ? tabs.length : gap }
}

/**
 * Pointer drags rather than HTML drag and drop: WKWebView claims a native drag
 * for file drops, and a Terminal or a document iframe under the pointer would
 * swallow its events. Capture keeps them coming across both. Without a native
 * drag there is no drag image either, so `dragging` and `pointer` let the host
 * draw one.
 */
export function usePanelDrag(panels: PanelDragTarget, host: Ref<HTMLElement | null>) {
  const tabDrop = ref<TabDrop | null>(null)
  /** The tab a press has carried past the drag threshold, until it is let go. */
  const dragging = ref<string | null>(null)
  const pointer = ref<{ x: number; y: number } | null>(null)

  function settle(): void {
    tabDrop.value = null
    dragging.value = null
    pointer.value = null
  }

  function startTabDrag(event: PointerEvent, tabId: string): void {
    if (event.button !== 0) return
    // Unclaimed, WebKit can read the press as the start of its own selection or
    // drag and cancel the pointer. The click that selects the tab still fires.
    event.preventDefault()
    const { clientX: startX, clientY: startY } = event
    startDrag(event, {
      pointerCapture: true,
      onMove: (move) => {
        if (!dragging.value && Math.hypot(move.clientX - startX, move.clientY - startY) < DRAG_THRESHOLD_PX) return
        dragging.value = tabId
        pointer.value = { x: move.clientX, y: move.clientY }
        tabDrop.value = dropAt(move.clientX, move.clientY)
      },
      onEnd: () => {
        const drop = tabDrop.value
        const tab = dragging.value
        settle()
        if (drop && tab) panels.moveTabTo(tab, drop.panel, drop.gap)
      },
      onCancel: settle,
    })
  }

  // Keeps the offset from the boundary to where it was grabbed, because a stacked
  // tab row is a grab area as deep as the row and the boundary must not jump to the pointer.
  function startResize(event: PointerEvent, split: PanelSplit, box: PanelBox): void {
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
      onMove: (move) => panels.apply((current) => resizeSplit(current, split.id, ratioAt(move) - offset)),
    })
  }

  function stepResize(split: PanelSplit, direction: number): void {
    panels.apply((current) => resizeSplit(current, split.id, split.ratio + direction * KEY_STEP))
  }

  return { tabDrop, dragging, pointer, startTabDrag, startResize, stepResize }
}
