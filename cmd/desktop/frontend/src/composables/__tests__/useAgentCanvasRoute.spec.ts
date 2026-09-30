import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createAppRouter } from '../../router'
import { useAgentCanvasRoute } from '../useAgentCanvasRoute'

// The composable resolves against a live route, so it is exercised through a
// throwaway host component rather than called bare.
type Api = ReturnType<typeof useAgentCanvasRoute>

async function mountAt(path: string): Promise<{ api: Api; router: Router }> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  let api!: Api
  const Host = defineComponent({
    setup() {
      api = useAgentCanvasRoute()
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { api, router }
}

describe('useAgentCanvasRoute', () => {
  it('reads the open chat and canvas off the route', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7&canvas=plan')
    expect(api.routeChatId.value).toBe(7)
    expect(api.canvasRequested.value).toBe(true)
    expect(api.canvasVisible.value).toBe(true)
    expect(api.canvasName.value).toBe('plan')
  })

  // A bare ?canvas means "you pick", so it carries no name.
  it('treats a bare ?canvas as an unnamed pick', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7&canvas=1')
    expect(api.canvasRequested.value).toBe(true)
    expect(api.canvasName.value).toBeNull()
  })

  // ?canvas names a view of the open chat, so with no chat it resolves to
  // nothing to show even though the query is present.
  it('is requested but not visible without an open chat', async () => {
    const { api } = await mountAt('/workspaces/web-app?canvas=1')
    expect(api.canvasRequested.value).toBe(true)
    expect(api.canvasVisible.value).toBe(false)
  })

  it('is inert outside the agents route', async () => {
    const { api, router } = await mountAt('/feed')
    expect(api.routeChatId.value).toBeNull()
    expect(api.canvasRequested.value).toBe(false)

    api.syncCanvasQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()
  })

  it('opens, names and closes the canvas without stacking history', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7')
    const depth = window.history.length

    api.syncCanvasQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBe('1')

    api.syncCanvasQuery(true, 'plan')
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBe('plan')

    api.syncCanvasQuery(false)
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()
    expect(window.history.length).toBe(depth)
  })

  // A bare open after a named one keeps the name rather than resetting the pick.
  it('keeps the pinned name on a bare reopen', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7&canvas=plan')
    api.syncCanvasQuery(false)
    await flushPromises()

    await router.replace({ query: { chat: '7', canvas: 'plan' } })
    await flushPromises()
    api.syncCanvasQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBe('plan')
  })

  // The dot is keyed by the authoring chat: a write to the chat in view with
  // its pane open is already read, a write to any other chat is remembered and
  // lights when that chat comes into view.
  it('keys the unseen dot by chat', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7')
    expect(api.canvasUnseen.value).toBe(false)

    api.noteCanvasWrite(9)
    expect(api.canvasUnseen.value).toBe(false)

    api.noteCanvasWrite(7)
    expect(api.canvasUnseen.value).toBe(true)

    api.clearCanvasUnseen(7)
    expect(api.canvasUnseen.value).toBe(false)

    // The write to 9 was remembered, so switching to it lights its own dot.
    await router.replace({ query: { chat: '9' } })
    await flushPromises()
    expect(api.canvasUnseen.value).toBe(true)
  })

  it('ignores a write while that chat has its canvas on screen', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7&canvas=1')
    api.noteCanvasWrite(7)
    expect(api.canvasUnseen.value).toBe(false)
  })

  it('ignores a write with no usable session id', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7')
    api.noteCanvasWrite(Number.NaN)
    api.noteCanvasWrite(0)
    api.noteCanvasWrite(-3)
    expect(api.canvasUnseen.value).toBe(false)
  })

  // The set is module state so the title bar and AgentsMode read one truth.
  it('shares the unseen set across callers', async () => {
    const { api: first, router } = await mountAt('/workspaces/web-app?chat=7')
    first.noteCanvasWrite(7)

    let second!: Api
    const Host = defineComponent({
      setup() {
        second = useAgentCanvasRoute()
        return () => null
      },
    })
    mount(Host, { global: { plugins: [router] } })
    await flushPromises()

    expect(second.canvasUnseen.value).toBe(true)
    second.clearCanvasUnseen(7)
    expect(first.canvasUnseen.value).toBe(false)
  })
})
