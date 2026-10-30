import { computed, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { useDriveRoute } from '../../composables/useDriveRoute'
import { useWailsEvent } from '../../composables/useWailsEvent'
import { driveEventAuthor, type DriveAuthor } from '../../lib/drive'

/** The space the attached session reads, and who it is as an author. */
export interface TerminalDriveTarget {
  space: string
  session: DriveAuthor
}

/**
 * The drive pane beside the session attached in Code. It rides ?doc on the
 * terminal route the way the Chats pane rides it on the agents route, so the
 * title bar's right toggle, a reload, and an agent's open_document reach it
 * the same way in both areas.
 */
export function useTerminalDrive(target: Ref<TerminalDriveTarget | null>, active: () => boolean) {
  const route = useRoute()
  const { docRequested, docPath, gitRequested, gitPath, syncDocQuery, syncGitQuery, closeSidePane } = useDriveRoute()

  const open = computed(() => route.name === 'terminal' && docRequested.value)
  const pane = computed(() => (open.value ? target.value : null))
  const paneOpen = computed(() => route.name === 'terminal' && (docRequested.value || gitRequested.value))

  // A route write outside useTerminalAttach can still rebuild the query. A
  // close or a tab switch changes nothing but these two queries, so any other
  // write that drops the open one gets it back, with the pinned path only while
  // the session is the same one.
  watch(
    () => ({
      slug: route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
      window: route.query.window,
      doc: route.name === 'terminal' ? route.query.doc : undefined,
      git: route.name === 'terminal' ? route.query.git : undefined,
    }),
    (now, before) => {
      if (!now.slug || !before.slug || now.doc !== undefined || now.git !== undefined) return
      const sameSession = now.slug === before.slug
      if (sameSession && now.window === before.window) return
      if (before.doc !== undefined) syncDocQuery(true, sameSession && typeof before.doc === 'string' ? before.doc : '1')
      else if (before.git !== undefined)
        syncGitQuery(true, sameSession && typeof before.git === 'string' ? before.git : '1')
    },
  )

  // An agent can ask to open or close the pane. Honored only for the session
  // in view, for the Chats pane's reason: it must never drag the user away.
  useWailsEvent('drive:toggle', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as
      { path?: string; open: boolean } | undefined
    if (!payload || !active() || route.name !== 'terminal') return
    if (driveEventAuthor(payload) !== (target.value?.session ?? null)) return
    syncDocQuery(payload.open, payload.path || undefined)
  })

  return { pane, paneOpen, docPath, gitPath, gitRequested, syncDocQuery, syncGitQuery, closeSidePane }
}
