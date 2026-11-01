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

export type SessionTabPlacement = 'behind' | 'beside' | 'below'

type WireView = 'drive' | 'diff' | 'tasks' | 'terminal'

/** A tab as the wire names it. A terminal with no window is the chat's own, in Chats. */
export interface TabRef {
  view: WireView
  window: string
}

interface SessionLayoutPayload {
  verb?: string
  view?: string
  window?: string
  anchorView?: string
  anchorWindow?: string
  path?: string
  placement?: string
  share?: number
}

// The wire names the Drive view "drive"; the frontend's kind for it is "docs".
const VIEW_OF_WIRE = new Map<string, SessionViewKind>([
  ['drive', 'docs'],
  ['diff', 'diff'],
  ['tasks', 'tasks'],
])

function tabRefOf(view: string | undefined, window: string | undefined): TabRef | null {
  if (view !== 'drive' && view !== 'diff' && view !== 'tasks' && view !== 'terminal') return null
  return { view, window: window ?? '' }
}

function placementOf(placement: string | undefined): SessionTabPlacement {
  return placement === 'beside' || placement === 'below' ? placement : 'behind'
}

/**
 * The ways into a session's layout from outside it. ?doc is how a reveal links
 * to a document, and an agent's layout verbs arrive as session:layout. The
 * Drive tab owns the document after a reveal, so the query is consumed: left in
 * place, it would follow the route into the next session. It acts only on the
 * route of the area that hosts the tabs, because both areas stay mounted.
 */
export function useSessionLayout(
  routeName: 'terminal' | 'agents',
  target: Ref<SessionDriveTarget | null>,
  active: () => boolean,
  handlers: {
    ready: () => boolean
    /** ?doc on the route: the user followed a link, so the Drive tab takes the screen. */
    reveal: (path: string | null) => void
    /** An agent's ask. It must not take the keyboard. A null anchor means the tab in front of the focused panel. */
    open: (view: SessionViewKind, path: string | null, placement: SessionTabPlacement, anchor: TabRef | null) => void
    /** An agent's terminal. The window tab itself arrives with tmux's window list; this says where it goes. */
    openWindow?: (windowId: string, placement: SessionTabPlacement, anchor: TabRef | null) => void
    close: (view: SessionViewKind) => void
    move: (tab: TabRef, placement: SessionTabPlacement, anchor: TabRef | null) => void
    focus: (tab: TabRef) => void
    closePanel: (tab: TabRef) => void
    resize: (tab: TabRef, share: number) => void
    turn: (tab: TabRef) => void
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
  useWailsEvent('session:layout', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as SessionLayoutPayload | undefined
    if (!payload || !active() || route.name !== routeName) return
    if (driveEventAuthor(payload) !== (target.value?.session ?? null)) return
    const tab = tabRefOf(payload.view, payload.window)
    if (!tab) return
    const placement = placementOf(payload.placement)
    const anchor = tabRefOf(payload.anchorView, payload.anchorWindow)

    switch (payload.verb) {
      case 'open': {
        if (tab.view === 'terminal') {
          if (tab.window) handlers.openWindow?.(tab.window, placement, anchor)
          return
        }
        handlers.open(VIEW_OF_WIRE.get(tab.view)!, payload.path || null, placement, anchor)
        return
      }
      case 'close':
        // Go kills the window itself, and its tab leaves with the window.
        if (tab.view !== 'terminal') handlers.close(VIEW_OF_WIRE.get(tab.view)!)
        return
      case 'move':
        handlers.move(tab, placement, anchor)
        return
      case 'focus':
        handlers.focus(tab)
        return
      case 'close_panel':
        handlers.closePanel(tab)
        return
      case 'resize':
        if (typeof payload.share === 'number' && Number.isFinite(payload.share)) handlers.resize(tab, payload.share)
        return
      case 'turn':
        handlers.turn(tab)
        return
      default:
        return
    }
  })
}
