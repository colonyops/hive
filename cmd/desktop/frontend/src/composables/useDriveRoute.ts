import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

// ?doc is how a reveal or a link asks a session's Drive tab to open, in Chats
// and in Code alike: bare, it means "you pick", and ?doc=<path> names one
// document. The tab consumes the query once it has opened (useSessionDrive), so
// the query is a request, not state. Everything is derived from the route, so
// each caller resolves it against its own useRoute().
export function useDriveRoute() {
  const route = useRoute()
  const router = useRouter()

  const routeChatId = computed(() => {
    if (route.name !== 'agents') return null
    const raw = route.query.chat
    return typeof raw === 'string' && raw ? raw : null
  })

  const onDriveRoute = computed(() => route.name === 'agents' || route.name === 'terminal')
  const docRequested = computed(() => onDriveRoute.value && route.query.doc !== undefined)
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

  return {
    routeChatId,
    docRequested,
    docPath,
    syncDocQuery,
  }
}
