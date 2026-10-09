import { computed, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { CanvasAuthor } from '../lib/agentCanvas'

// The canvas pane rides the route in Chats and in Code alike: ?canvas opens it
// beside the chat or session the route names, and ?canvas=<name> pins one
// canvas. Three surfaces read and write that query — AgentsMode and
// TerminalMode, which render the pane, and the title bar's right-panel toggle,
// which opens and closes it — so the query logic lives here rather than in any
// of them. Independent writers of one query param drift.
//
// Only the unseen set is module state: it has to survive a caller unmounting
// and be the same set for all of them. Everything else is derived from the
// route, so each caller resolves it against its own useRoute().

// An agent wrote to a canvas the user was not looking at. Keyed by the author,
// a chat or a hive session, so a write in the background lights its dot on
// return and a switch never inherits another's dot. Content is never carried
// here — opening the pane reads it.
const unseenCanvasAuthors = reactive(new Set<CanvasAuthor>())

export function useAgentCanvasRoute() {
  const route = useRoute()
  const router = useRouter()

  const routeChatId = computed(() => {
    if (route.name !== 'agents') return null
    const raw = route.query.chat
    return typeof raw === 'string' && raw ? raw : null
  })

  const onCanvasRoute = computed(() => route.name === 'agents' || route.name === 'terminal')
  const canvasRequested = computed(() => onCanvasRoute.value && route.query.canvas !== undefined)
  // The Chats pane is only shown beside an open chat, so this — not
  // canvasRequested — is what its title-bar toggle keys its enabled state on.
  const canvasVisible = computed(() => route.name === 'agents' && canvasRequested.value && routeChatId.value !== null)
  const canvasName = computed<string | null>(() => {
    const raw = route.query.canvas
    return typeof raw === 'string' && raw !== '' && raw !== '1' ? raw : null
  })

  // Written with replace so history never stacks. A bare open (no name) keeps
  // whatever name the query already carried, falling back to '1' — "you pick".
  function syncCanvasQuery(open: boolean, name?: string | null): void {
    if (!onCanvasRoute.value) return
    const next = open
      ? name === null
        ? '1'
        : (name ?? (typeof route.query.canvas === 'string' && route.query.canvas !== '' ? route.query.canvas : '1'))
      : undefined
    if (route.query.canvas === next) return
    void router.replace({ name: route.name, params: route.params, query: { ...route.query, canvas: next } })
  }

  const canvasUnseen = computed(() => routeChatId.value !== null && unseenCanvasAuthors.has(routeChatId.value))

  function noteCanvasWrite(session: string): void {
    if (!session) return
    if (canvasVisible.value && session === routeChatId.value) return
    unseenCanvasAuthors.add(session)
  }

  function markCanvasUnseen(author: CanvasAuthor): void {
    unseenCanvasAuthors.add(author)
  }

  function clearCanvasUnseen(author: CanvasAuthor): void {
    unseenCanvasAuthors.delete(author)
  }

  function isCanvasUnseen(author: CanvasAuthor | null): boolean {
    return author !== null && unseenCanvasAuthors.has(author)
  }

  return {
    routeChatId,
    canvasRequested,
    canvasVisible,
    canvasName,
    canvasUnseen,
    syncCanvasQuery,
    noteCanvasWrite,
    markCanvasUnseen,
    clearCanvasUnseen,
    isCanvasUnseen,
  }
}
