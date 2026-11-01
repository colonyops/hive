import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createAppRouter } from '../../router'
import { useDriveRoute } from '../useDriveRoute'

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
    expect(api.docPath.value).toBe('plans/x.md')
  })

  // A bare ?doc means "you pick", so it carries no name.
  it('treats a bare ?doc as an unnamed pick', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7&doc=1')
    expect(api.docRequested.value).toBe(true)
    expect(api.docPath.value).toBeNull()
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

  // Code rides the same query on its own route.
  it('opens and closes the pane on the terminal route, keeping the session and window', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?window=%402')
    expect(api.docRequested.value).toBe(false)

    api.syncDocQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('terminal')
    expect(router.currentRoute.value.params.slug).toBe('fix-login')
    expect(router.currentRoute.value.query).toEqual({ window: '@2', doc: '1' })
    expect(api.docRequested.value).toBe(true)

    api.syncDocQuery(false)
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ window: '@2' })
  })
})
