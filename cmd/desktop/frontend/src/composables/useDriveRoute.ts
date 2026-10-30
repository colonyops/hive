import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDriveActivity } from '../stores/useDriveActivity'

// The drive pane rides the route in Chats and in Code alike: ?doc opens it
// beside the chat or session the route names, and ?doc=<path> pins one
// document. AgentsMode, TerminalMode and the title bar's right-panel toggle
// all read and write that query, so its logic lives here: independent writers
// of one query param drift. Everything is derived from the route, so each
// caller resolves it against its own useRoute().
export function useDriveRoute() {
  const route = useRoute()
  const router = useRouter()
  const activity = useDriveActivity()

  const routeChatId = computed(() => {
    if (route.name !== 'agents') return null
    const raw = route.query.chat
    return typeof raw === 'string' && raw ? raw : null
  })

  const onDriveRoute = computed(() => route.name === 'agents' || route.name === 'terminal')
  const docRequested = computed(() => onDriveRoute.value && route.query.doc !== undefined)
  // The Chats pane is only shown beside an open chat, so this, not
  // docRequested, is what its title-bar toggle keys its enabled state on.
  const docVisible = computed(() => route.name === 'agents' && docRequested.value && routeChatId.value !== null)
  const docPath = computed<string | null>(() => {
    const raw = route.query.doc
    return typeof raw === 'string' && raw !== '' && raw !== '1' ? raw : null
  })

  // Written with replace so history never stacks. A bare open (no path) keeps
  // whatever path the query already carried, falling back to '1': "you pick".
  function syncDocQuery(open: boolean, path?: string | null): void {
    if (!onDriveRoute.value) return
    const current = typeof route.query.doc === 'string' && route.query.doc !== '' ? route.query.doc : '1'
    const next = open ? (path === null ? '1' : (path ?? current)) : undefined
    if (route.query.doc === next) return
    void router.replace({ name: route.name, params: route.params, query: { ...route.query, doc: next } })
  }

  const docUnseen = computed(() => activity.isUnseen(routeChatId.value))

  return {
    routeChatId,
    docRequested,
    docVisible,
    docPath,
    docUnseen,
    syncDocQuery,
  }
}
