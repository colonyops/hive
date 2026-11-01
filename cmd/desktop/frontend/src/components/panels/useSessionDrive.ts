import { watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { useDriveRoute } from '../../composables/useDriveRoute'
import { useWailsEvent } from '../../composables/useWailsEvent'
import { driveEventAuthor, type DriveAuthor } from '../../lib/drive'

/** The space the open session reads, and who it is as an author. */
export interface SessionDriveTarget {
  space: string
  session: DriveAuthor
}

/**
 * The ways into a session's Drive tab from outside it. ?doc is how a reveal
 * links to a document, and an agent's open_document arrives as drive:toggle.
 * The tab owns the document after that, so the query is consumed: left in
 * place, it would follow the route into the next session. It acts only on
 * the route of the area that hosts the tabs, because both areas stay mounted.
 */
export function useSessionDrive(
  routeName: 'terminal' | 'agents',
  target: Ref<SessionDriveTarget | null>,
  active: () => boolean,
  docs: {
    ready: () => boolean
    open: (path: string | null) => void
    /** An agent's document: it must not take the screen from what the user is doing. */
    arrive: (path: string | null) => void
    close: () => void
  },
) {
  const route = useRoute()
  const { docRequested, docPath, syncDocQuery } = useDriveRoute()

  watch(
    () => route.name === routeName && docRequested.value && !!target.value && docs.ready(),
    (requested) => {
      if (!requested) return
      docs.open(docPath.value)
      syncDocQuery(false)
    },
    { immediate: true },
  )

  // Honored only for the session in view: an agent must never drag the user away.
  useWailsEvent('drive:toggle', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as
      { path?: string; open: boolean } | undefined
    if (!payload || !active() || route.name !== routeName) return
    if (driveEventAuthor(payload) !== (target.value?.session ?? null)) return
    if (payload.open) docs.arrive(payload.path || null)
    else docs.close()
  })
}
