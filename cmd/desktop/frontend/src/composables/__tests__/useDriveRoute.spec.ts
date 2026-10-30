import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../router'
import { useDriveRoute } from '../useDriveRoute'

const events = vi.hoisted(() => ({
  handlers: new Map<string, (event: { data: unknown }) => void>(),
}))
vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, handler: (event: { data: unknown }) => void) => {
      events.handlers.set(name, handler)
      return () => {}
    },
  },
}))

function agentWrote(session: string): void {
  events.handlers.get('drive:updated')?.({ data: { space: 'workspaces/web-app', paths: ['x.md'], session } })
}

// The composable resolves against a live route, so it is exercised through a
// throwaway host component rather than called bare.
type Api = ReturnType<typeof useDriveRoute>

async function mountAt(path: string): Promise<{ api: Api; router: Router }> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  let api!: Api
  const Host = defineComponent({
    setup() {
      api = useDriveRoute()
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { api, router }
}

describe('useDriveRoute', () => {
  it('reads the open chat and document off the route', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7&doc=plans%2Fx.md')
    expect(api.routeChatId.value).toBe('7')
    expect(api.docRequested.value).toBe(true)
    expect(api.docVisible.value).toBe(true)
    expect(api.docPath.value).toBe('plans/x.md')
  })

  // A bare ?doc means "you pick", so it carries no name.
  it('treats a bare ?doc as an unnamed pick', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7&doc=1')
    expect(api.docRequested.value).toBe(true)
    expect(api.docPath.value).toBeNull()
  })

  // ?doc names a view of the open chat, so with no chat it resolves to
  // nothing to show even though the query is present.
  it('is requested but not visible without an open chat', async () => {
    const { api } = await mountAt('/workspaces/web-app?doc=1')
    expect(api.docRequested.value).toBe(true)
    expect(api.docVisible.value).toBe(false)
  })

  it('is inert outside the agents route', async () => {
    const { api, router } = await mountAt('/feed')
    expect(api.routeChatId.value).toBeNull()
    expect(api.docRequested.value).toBe(false)

    api.syncDocQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBeUndefined()
  })

  it('opens, names and closes the pane without stacking history', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7')
    const depth = window.history.length

    api.syncDocQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBe('1')

    api.syncDocQuery(true, 'plans/x.md')
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBe('plans/x.md')

    api.syncDocQuery(false)
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBeUndefined()
    expect(window.history.length).toBe(depth)
  })

  // A bare open after a named one keeps the path rather than resetting the pick.
  it('keeps the pinned path on a bare reopen', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7&doc=plans%2Fx.md')
    api.syncDocQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBe('plans/x.md')
  })

  // The dot is keyed by the authoring chat, so switching to a chat lights
  // only its own.
  it('reads the unseen dot of the chat the route names', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7')
    expect(api.docUnseen.value).toBe(false)

    agentWrote('9')
    expect(api.docUnseen.value).toBe(false)

    agentWrote('7')
    expect(api.docUnseen.value).toBe(true)

    await router.replace({ query: { chat: '9' } })
    await flushPromises()
    expect(api.docUnseen.value).toBe(true)
  })

  // Code rides the same query on its own route, so the title bar's toggle and
  // a reload reach its pane the way they reach the Chats one.
  it('opens and closes the pane on the terminal route, keeping the session and window', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?window=%402')
    expect(api.docRequested.value).toBe(false)

    api.syncDocQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('terminal')
    expect(router.currentRoute.value.params.slug).toBe('fix-login')
    expect(router.currentRoute.value.query).toEqual({ window: '@2', doc: '1' })
    expect(api.docRequested.value).toBe(true)
    expect(api.docVisible.value).toBe(false)

    api.syncDocQuery(false)
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ window: '@2' })
  })

  it('opens Git on the terminal route, and opening either query drops the other', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?window=%402&doc=plans%2Fx.md')

    api.syncGitQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ window: '@2', git: '1' })
    expect(api.gitRequested.value).toBe(true)
    expect(api.docRequested.value).toBe(false)
    expect(api.gitPath.value).toBeNull()

    api.syncGitQuery(true, 'cmd/a.go')
    await flushPromises()
    expect(api.gitPath.value).toBe('cmd/a.go')

    api.syncDocQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ window: '@2', doc: '1' })
    expect(api.gitRequested.value).toBe(false)
  })

  it('keeps the file on a bare reopen of Git', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?git=cmd%2Fa.go')
    api.syncGitQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.git).toBe('cmd/a.go')
  })

  it('closes whichever pane is open', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?window=%402&git=1')
    api.closeSidePane()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ window: '@2' })
  })

  it('has no Git pane in Chats', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7')
    expect(api.gitRequested.value).toBe(false)

    api.syncGitQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.git).toBeUndefined()
  })
})
