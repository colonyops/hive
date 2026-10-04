import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent, ref, type Ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../../router'
import { useAgentCanvasRoute } from '../../../composables/useAgentCanvasRoute'
import { useTerminalCanvas, type TerminalCanvasTarget } from '../useTerminalCanvas'

const wailsEvents = vi.hoisted(() => ({
  handlers: [] as Array<[string, (event: { data: unknown }) => void]>,
  fire(name: string, data: unknown) {
    for (const [registered, handler] of this.handlers) {
      if (registered === name) handler({ data })
    }
  },
}))
vi.mock('../../../composables/useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: (event: { data: unknown }) => void) => {
    wailsEvents.handlers.push([name, handler])
  },
}))

const agents = vi.hoisted(() => ({ Available: vi.fn() }))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: agents.Available,
}))

type Api = ReturnType<typeof useTerminalCanvas>
const session: TerminalCanvasTarget = { workspace: 'acme/site', session: 'abc123' }

async function mountAt(
  path: string,
  initial: TerminalCanvasTarget | null = session,
): Promise<{ api: Api; router: Router; target: Ref<TerminalCanvasTarget | null>; active: Ref<boolean> }> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  const target = ref<TerminalCanvasTarget | null>(initial)
  const active = ref(true)
  let api!: Api
  const Host = defineComponent({
    setup() {
      api = useTerminalCanvas(target, () => active.value)
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { api, router, target, active }
}

describe('useTerminalCanvas', () => {
  beforeEach(() => {
    wailsEvents.handlers = []
    agents.Available.mockResolvedValue({ available: false, reason: 'gated off' })
  })

  it('shows the pane for the attached session while the route asks for it', async () => {
    const { api, router } = await mountAt('/terminal/fix-login')
    expect(api.pane.value).toBeNull()

    await router.replace('/terminal/fix-login?canvas=plan')
    expect(api.pane.value).toEqual(session)
    expect(api.canvasName.value).toBe('plan')
  })

  // The scratch terminal has no checkout, so it has no canvases to show.
  it('shows nothing for a session with no canvases, whatever the route says', async () => {
    const { api } = await mountAt('/terminal/Scratch?canvas=1', null)
    expect(api.pane.value).toBeNull()
  })

  it('keeps the pane open across a session switch, and drops the pinned name', async () => {
    const { api, router, target } = await mountAt('/terminal/fix-login?canvas=plan')

    target.value = { workspace: 'acme/api', session: 'def456' }
    await router.push('/terminal/add-search')
    await flushPromises()

    expect(router.currentRoute.value.query.canvas).toBe('1')
    expect(api.pane.value).toEqual({ workspace: 'acme/api', session: 'def456' })
    expect(api.canvasName.value).toBeNull()
  })

  // The attach mirrors the active window into the query and rebuilds it to do
  // so. That is not a close.
  it('keeps the pane and its pinned canvas when the window changes under it', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?canvas=plan')

    await router.replace({ name: 'terminal', params: { slug: 'fix-login' }, query: { window: '@2' } })
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ window: '@2', canvas: 'plan' })
    expect(api.pane.value).toEqual(session)
  })

  it('closes when only the canvas query is dropped', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?window=%402&canvas=plan')

    api.syncCanvasQuery(false)
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ window: '@2' })
    expect(api.pane.value).toBeNull()
  })

  it('leaves a closed pane closed across a session switch', async () => {
    const { router } = await mountAt('/terminal/fix-login')

    await router.push('/terminal/add-search')
    await flushPromises()

    expect(router.currentRoute.value.query.canvas).toBeUndefined()
  })

  it('opens and closes on canvas:toggle for the session in view only', async () => {
    const { router, active } = await mountAt('/terminal/fix-login')

    wailsEvents.fire('canvas:toggle', { session: 0, hiveSession: 'def456', name: '', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()

    active.value = false
    wailsEvents.fire('canvas:toggle', { session: 0, hiveSession: 'abc123', name: 'plan', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()

    active.value = true
    wailsEvents.fire('canvas:toggle', { session: 0, hiveSession: 'abc123', name: 'plan', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBe('plan')

    wailsEvents.fire('canvas:toggle', { session: 0, hiveSession: 'abc123', name: '', open: false })
    await flushPromises()
    expect(router.currentRoute.value.query.canvas).toBeUndefined()
  })

  it('marks a write unseen until the pane for that session shows it', async () => {
    const { router } = await mountAt('/terminal/fix-login')
    const { isCanvasUnseen } = useCanvasRouteFor(router)

    wailsEvents.fire('canvas:updated', { session: 0, hiveSession: 'abc123' })
    await flushPromises()
    expect(isCanvasUnseen('abc123')).toBe(true)

    await router.replace('/terminal/fix-login?canvas=1')
    await flushPromises()
    expect(isCanvasUnseen('abc123')).toBe(false)

    wailsEvents.fire('canvas:updated', { session: 0, hiveSession: 'abc123' })
    await flushPromises()
    expect(isCanvasUnseen('abc123')).toBe(false)
  })
})

// The unseen set is shared module state, read here through a second caller the
// way the title bar's toggle reads it.
function useCanvasRouteFor(router: Router): ReturnType<typeof useAgentCanvasRoute> {
  let api!: ReturnType<typeof useAgentCanvasRoute>
  const Host = defineComponent({
    setup() {
      api = useAgentCanvasRoute()
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  return api
}
