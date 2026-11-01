import { watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { useDriveRoute } from '../../composables/useDriveRoute'
import { useWailsEvent } from '../../composables/useWailsEvent'
import { driveEventAuthor, type DriveAuthor } from '../../lib/drive'
import type { SessionViewKind } from '../terminal/panelViews'

/** The space the open session reads, and who it is as an author. */
export interface SessionDriveTarget {
  space: string
  session: DriveAuthor
}

export type SessionTabPlacement = 'behind' | 'beside'

interface SessionTabPayload {
  view?: string
  path?: string
  placement?: string
  open: boolean
}

// The wire names the Drive view "drive"; the frontend's kind for it is "docs".
const VIEW_OF_WIRE = new Map<string, SessionViewKind>([
  ['drive', 'docs'],
  ['diff', 'diff'],
  ['tasks', 'tasks'],
])

/**
 * The ways into a session's view tabs from outside them. ?doc is how a reveal
 * links to a document, and an agent's open_tab and close_tab arrive as
 * session:tab. The Drive tab owns the document after a reveal, so the query is
 * consumed: left in place, it would follow the route into the next session. It
 * acts only on the route of the area that hosts the tabs, because both areas
 * stay mounted.
 */
export function useSessionTabs(
  routeName: 'terminal' | 'agents',
  target: Ref<SessionDriveTarget | null>,
  active: () => boolean,
  handlers: {
    ready: () => boolean
    /** ?doc on the route: the user followed a link, so the Drive tab takes the screen. */
    reveal: (path: string | null) => void
    /** An agent's ask. It must not take the keyboard. */
    open: (view: SessionViewKind, path: string | null, placement: SessionTabPlacement) => void
    close: (view: SessionViewKind) => void
  },
) {
  const route = useRoute()
  const { docRequested, docPath, syncDocQuery } = useDriveRoute()

  watch(
    () => route.name === routeName && docRequested.value && !!target.value && handlers.ready(),
    (requested) => {
      if (!requested) return
      handlers.reveal(docPath.value)
      syncDocQuery(false)
    },
    { immediate: true },
  )

  // Honored only for the session in view: an agent must never drag the user away.
  useWailsEvent('session:tab', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as SessionTabPayload | undefined
    if (!payload || !active() || route.name !== routeName) return
    if (driveEventAuthor(payload) !== (target.value?.session ?? null)) return
    const view = VIEW_OF_WIRE.get(payload.view ?? '')
    if (!view) return
    if (payload.open) handlers.open(view, payload.path || null, payload.placement === 'beside' ? 'beside' : 'behind')
    else handlers.close(view)
  })
}
