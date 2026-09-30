import { computed, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'

// The Chats canvas pane rides the route (?chat names the open chat, ?canvas
// names its canvas). Two surfaces read and write that query — AgentsMode, which
// renders the pane, and the title bar's right-panel toggle, which opens and
// closes it — so the query logic lives here rather than in either of them. Two
// independent writers of one query param drift.
//
// Only the unseen set is module state: it has to survive a caller unmounting
// and be the same set for both. Everything else is derived from the route, so
// each caller resolves it against its own useRoute().

// An agent wrote to a chat's canvas the user was not looking at. Keyed by the
// authoring session so a write to a background chat lights its dot on return
// and a chat switch never inherits another chat's dot. Content is never carried
// here — opening the pane reads it.
const unseenCanvasSessions = reactive(new Set<number>())

export function useAgentCanvasRoute() {
  const route = useRoute()
  const router = useRouter()

  const routeChatId = computed(() => {
    if (route.name !== 'agents') return null
    const raw = route.query.chat
    const id = typeof raw === 'string' ? Number.parseInt(raw, 10) : Number.NaN
    return Number.isInteger(id) && id > 0 ? id : null
  })

  const canvasRequested = computed(() => route.name === 'agents' && route.query.canvas !== undefined)
  // The pane is only shown beside an open chat, so this — not canvasRequested —
  // is what the title-bar toggle keys its enabled state on.
  const canvasVisible = computed(() => canvasRequested.value && routeChatId.value !== null)
  const canvasName = computed<string | null>(() => {
    const raw = route.query.canvas
    return typeof raw === 'string' && raw !== '' && raw !== '1' ? raw : null
  })

  // Written with replace so history never stacks. A bare open (no name) keeps
  // whatever name the query already carried, falling back to '1' — "you pick".
  function syncCanvasQuery(open: boolean, name?: string): void {
    if (route.name !== 'agents') return
    const next = open ? (name ?? (typeof route.query.canvas === 'string' && route.query.canvas !== '' ? route.query.canvas : '1')) : undefined
    if (route.query.canvas === next) return
    void router.replace({ name: 'agents', params: route.params, query: { ...route.query, canvas: next } })
  }

  const canvasUnseen = computed(() => routeChatId.value !== null && unseenCanvasSessions.has(routeChatId.value))

  function noteCanvasWrite(session: number): void {
    if (!Number.isInteger(session) || session <= 0) return
    if (canvasVisible.value && session === routeChatId.value) return
    unseenCanvasSessions.add(session)
  }

  function clearCanvasUnseen(session: number): void {
    unseenCanvasSessions.delete(session)
  }

  return {
    routeChatId,
    canvasRequested,
    canvasVisible,
    canvasName,
    canvasUnseen,
    syncCanvasQuery,
    noteCanvasWrite,
    clearCanvasUnseen,
  }
}
