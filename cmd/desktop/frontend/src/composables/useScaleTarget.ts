import { useEventListener } from '@vueuse/core'
import type { Surface } from '../stores/useSurfaceScale'

const surfaceSelector = '[data-scale-surface]'
// An overlay like the palette opens over a surface and hands focus back to it,
// so working in one says nothing about which surface the person is looking at.
// A surface inside an overlay still counts: Changes and the pop-up are dialogs.
const overlaySelector = '[role="dialog"], [role="menu"], [role="listbox"]'
// When the last surface has gone, what is on screen decides: a terminal is the
// main thing in any view that draws one.
const fallbackOrder: Surface[] = ['terminal', 'document', 'code']

function isShown(element: Element): boolean {
  return element.isConnected && element.getClientRects().length > 0
}

function surfaceOf(element: Element): Surface {
  return element.getAttribute('data-scale-surface') as Surface
}

function surfaceOnScreen(): Surface {
  for (const surface of fallbackOrder) {
    for (const element of document.querySelectorAll(`[data-scale-surface="${surface}"]`)) {
      if (isShown(element)) return surface
    }
  }
  return 'interface'
}

/**
 * Which surface the text size chords act on: the last one clicked or focused.
 * Pointer and focus both count because a diff or a document is read without
 * ever taking focus. Clicking anywhere outside a surface means the interface.
 */
export function useScaleTarget(): { scaleTarget: () => Surface } {
  let last: Element | 'interface' | null = null

  function track(event: Event): void {
    if (!(event.target instanceof Element)) return
    const surface = event.target.closest(surfaceSelector)
    if (surface) last = surface
    else if (!event.target.closest(overlaySelector)) last = 'interface'
  }

  useEventListener(document, 'pointerdown', track, { capture: true })
  useEventListener(document, 'focusin', track, { capture: true })

  function scaleTarget(): Surface {
    if (last === 'interface') return 'interface'
    return last && isShown(last) ? surfaceOf(last) : surfaceOnScreen()
  }

  return { scaleTarget }
}
