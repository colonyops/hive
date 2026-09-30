import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { ComputedRef, Ref } from 'vue'

// A popover teleported to <body> and positioned from its anchor's bounding
// rect. Every call site sits inside something that clips — the actions drawer
// and the node editor both scroll, the session dialogs are modals — so an
// `absolute` popover inside a `relative` root would be cut off by the nearest
// `overflow` ancestor. Fixed positioning off body escapes all of them; the cost
// is repositioning on scroll/resize while open.

const GAP = 6
const EDGE = 8
const MAX_HEIGHT = 320
const MIN_HEIGHT = 140

export interface AnchoredPopover {
  style: ComputedRef<Record<string, string>>
  /** Re-read the anchor and popover rects. Call after opening and after the list re-renders. */
  measure: () => void
}

export function useAnchoredPopover(
  anchor: Ref<HTMLElement | null>,
  popover: Ref<HTMLElement | null>,
  open: Ref<boolean>,
): AnchoredPopover {
  const placement = ref({ left: 0, minWidth: 0, maxWidth: 0, top: 0, bottom: 0, flip: false, maxHeight: MAX_HEIGHT })

  function measure(): void {
    const el = anchor.value
    if (!el) return
    const rect = el.getBoundingClientRect()
    const viewport = window.innerHeight
    const below = viewport - rect.bottom - GAP - EDGE
    const above = rect.top - GAP - EDGE
    const flip = below < MIN_HEIGHT && above > below
    // The list is at least as wide as the anchor but grows past it rather than
    // truncating a long label ("succ…"), so it can stick out to the right — and
    // shifts back left once that would run off the viewport. Measured from the
    // rendered popover, so opening runs this twice: once to place it, once with
    // its real width.
    const width = popover.value?.getBoundingClientRect().width ?? rect.width
    placement.value = {
      left: Math.max(EDGE, Math.min(rect.left, window.innerWidth - EDGE - width)),
      minWidth: rect.width,
      maxWidth: window.innerWidth - EDGE * 2,
      top: rect.bottom + GAP,
      bottom: viewport - rect.top + GAP,
      flip,
      maxHeight: Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, flip ? above : below)),
    }
  }

  const style = computed(() => ({
    left: `${placement.value.left}px`,
    minWidth: `${placement.value.minWidth}px`,
    maxWidth: `${placement.value.maxWidth}px`,
    maxHeight: `${placement.value.maxHeight}px`,
    ...(placement.value.flip ? { bottom: `${placement.value.bottom}px` } : { top: `${placement.value.top}px` }),
  }))

  function bind(): void {
    // Capture phase: scrolling any ancestor moves the anchor, not just window.
    window.addEventListener('scroll', measure, true)
    window.addEventListener('resize', measure)
  }
  function unbind(): void {
    window.removeEventListener('scroll', measure, true)
    window.removeEventListener('resize', measure)
  }

  watch(open, (isOpen) => (isOpen ? bind() : unbind()))
  onBeforeUnmount(unbind)

  return { style, measure }
}
