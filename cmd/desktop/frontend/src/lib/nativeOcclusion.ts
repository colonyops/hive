// Native terminal surfaces sit above the page, so anything the page floats
// over a terminal (a menu, a select's list, a dialog, a tooltip) would be
// hidden behind them. This finds those elements; each surface cuts their
// rectangles out so the page shows through.

/** Floating UI by role, wherever it renders. */
const FLOATING = '[role="menu"],[role="listbox"],[role="dialog"],[role="alertdialog"],[role="tooltip"]'

export interface OccluderRect {
  x: number
  y: number
  w: number
  h: number
}

/** The visible floating elements: by role, and everything teleported onto <body> beside the app. */
export function findOccluders(doc: Document = document): OccluderRect[] {
  const elements = new Set<Element>(doc.querySelectorAll(FLOATING))
  const app = doc.getElementById('app')
  for (const child of Array.from(doc.body.children)) {
    if (child !== app && child.tagName !== 'SCRIPT' && child.tagName !== 'STYLE') elements.add(child)
  }
  const rects: OccluderRect[] = []
  for (const element of elements) {
    const box = element.getBoundingClientRect()
    if (box.width <= 0 || box.height <= 0) continue
    rects.push({ x: Math.floor(box.left), y: Math.floor(box.top), w: Math.ceil(box.width), h: Math.ceil(box.height) })
  }
  return rects
}

/**
 * Reports the floating elements to `report` whenever they may have changed,
 * at most once a frame, and only when the set differs. Returns a stop function.
 */
export function watchOccluders(report: (rects: OccluderRect[]) => void, doc: Document = document): () => void {
  let frame = 0
  let last = ''
  const measure = (): void => {
    frame = 0
    const rects = findOccluders(doc)
    const key = JSON.stringify(rects)
    if (key === last) return
    last = key
    report(rects)
  }
  const schedule = (): void => {
    if (!frame) frame = requestAnimationFrame(measure)
  }
  const observer = new MutationObserver(schedule)
  observer.observe(doc.body, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['style', 'class', 'hidden', 'role', 'open'],
  })
  const view = doc.defaultView
  view?.addEventListener('resize', schedule)
  view?.addEventListener('scroll', schedule, true)
  schedule()
  return () => {
    observer.disconnect()
    view?.removeEventListener('resize', schedule)
    view?.removeEventListener('scroll', schedule, true)
    if (frame) cancelAnimationFrame(frame)
  }
}
