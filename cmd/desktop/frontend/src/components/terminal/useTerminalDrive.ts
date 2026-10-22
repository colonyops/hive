import { computed, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { useDriveRoute } from '../../composables/useDriveRoute'
import type { DriveAuthor } from '../../lib/drive'
import { createLogger } from '../../lib/log'

const log = createLogger('terminal-drive')

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
export function useTerminalDrive(target: Ref<TerminalDriveTarget | null>) {
  const route = useRoute()
  const { docRequested, docPath, syncDocQuery } = useDriveRoute()

  const open = computed(() => route.name === 'terminal' && docRequested.value)
  const pane = computed(() => (open.value ? target.value : null))

  // A route write outside useTerminalAttach can still rebuild the query. A
  // close changes nothing but ?doc, so any other write that drops it gets it
  // back, with the pinned path only while the session is the same one.
  watch(
    () => ({
      slug: route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
      window: route.query.window,
      doc: route.name === 'terminal' ? route.query.doc : undefined,
    }),
    (now, before) => {
      if (!now.slug || !before.slug || before.doc === undefined || now.doc !== undefined) return
      const sameSession = now.slug === before.slug
      if (sameSession && now.window === before.window) return
      void syncDocQuery(true, sameSession && typeof before.doc === 'string' ? before.doc : '1').catch(
        (error: unknown) => log.warn('Could not restore Drive pane', error),
      )
    },
  )

  return { pane, docPath, syncDocQuery }
}
