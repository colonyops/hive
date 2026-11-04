import { useEventListener } from '@vueuse/core'
import { errorText } from '../lib/appError'
import { Open as OpenLink } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/linkservice'

const HIVE_LINK = /^hive:\/\//i

/** The hive:// link a click landed on, looking through shadow roots, or null. */
export function hiveLinkOf(event: MouseEvent): string | null {
  for (const node of event.composedPath()) {
    if (node instanceof HTMLAnchorElement) {
      const href = node.getAttribute('href') ?? ''
      return HIVE_LINK.test(href) ? href : null
    }
  }
  return null
}

/**
 * Opens a hive:// link clicked anywhere in the app in place, the way a link
 * from the operating system opens. It listens in the capture phase so every
 * surface (agent reviews, the Drive reader, task and GitHub bodies) gets it
 * without a branch of its own, before a surface's guard drops the link as
 * not openable. Handing it to the OS instead would raise whichever app
 * registered the scheme, which in a dev build is the installed one.
 */
export function useHiveLinkClicks(showError: (message: string) => void): void {
  useEventListener(
    document,
    'click',
    (event: MouseEvent) => {
      const href = hiveLinkOf(event)
      if (!href) return
      event.preventDefault()
      event.stopPropagation()
      OpenLink(href).catch((err: unknown) => showError(errorText(err, 'Could not open the link.')))
    },
    { capture: true },
  )
}
