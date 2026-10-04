import { computed, nextTick, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAgentCanvasRoute } from '../../composables/useAgentCanvasRoute'
import { useWailsEvent } from '../../composables/useWailsEvent'
import { canvasEventAuthor, type CanvasAuthor } from '../../lib/agentCanvas'
import { useAgentWorkspaces } from '../../stores/useAgentWorkspaces'

/** Whose canvases the attached session reads, and who it is as their author. */
export interface TerminalCanvasTarget {
  workspace: string
  session: CanvasAuthor
}

/**
 * The canvas pane beside the session attached in Code. It rides ?canvas on the
 * terminal route the way the Chats pane rides it on the agents route, so the
 * title bar's right toggle, a reload, and an agent's open_canvas reach it the
 * same way in both areas.
 */
export function useTerminalCanvas(target: Ref<TerminalCanvasTarget | null>, active: () => boolean) {
  const route = useRoute()
  const { canvasRequested, canvasName, syncCanvasQuery, markCanvasUnseen, clearCanvasUnseen } = useAgentCanvasRoute()
  const { client, ready } = useAgentWorkspaces()

  const open = computed(() => route.name === 'terminal' && canvasRequested.value)
  const pane = computed(() => (open.value ? target.value : null))

  watch(
    pane,
    (shown) => {
      if (!shown) return
      clearCanvasUnseen(shown.session)
      // The canvas reads go through the Chats area's client, which nothing
      // else in Code is certain to have asked for yet.
      void ready()
    },
    { immediate: true },
  )

  // Code's own route writes rebuild the query: the active window's mirror, a
  // rename, a switch to another session. A close changes nothing but ?canvas,
  // so any other write that drops it gets it back, with the pinned name only
  // while the session is the same one.
  watch(
    () => ({
      slug: route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
      window: route.query.window,
      canvas: route.name === 'terminal' ? route.query.canvas : undefined,
    }),
    (now, before) => {
      if (!now.slug || !before.slug || before.canvas === undefined || now.canvas !== undefined) return
      const sameSession = now.slug === before.slug
      if (sameSession && now.window === before.window) return
      syncCanvasQuery(true, sameSession && typeof before.canvas === 'string' ? before.canvas : '1')
    },
  )

  useWailsEvent('canvas:updated', (event) => {
    const author = canvasEventAuthor(event.data)
    if (author === null) return
    // After every other listener: the Chats area marks a pinned chat's write
    // unseen, and the pane on screen here has already shown it.
    void nextTick(() => {
      if (pane.value?.session === author) clearCanvasUnseen(author)
      else if (typeof author === 'string') markCanvasUnseen(author)
    })
  })

  // An agent can ask to open or close the pane. Honored only for the session
  // in view, for the Chats pane's reason: it must never drag the user away.
  useWailsEvent('canvas:toggle', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as
      { name: string; open: boolean } | undefined
    if (!payload || !active() || route.name !== 'terminal') return
    if (canvasEventAuthor(payload) !== (target.value?.session ?? null)) return
    syncCanvasQuery(payload.open, payload.name || undefined)
  })

  return { pane, canvasName, client, syncCanvasQuery }
}
