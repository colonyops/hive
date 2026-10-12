import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent, ref, type Ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../../router'
import { useDriveRoute } from '../../../composables/useDriveRoute'
import { useTerminalDrive, type TerminalDriveTarget } from '../useTerminalDrive'

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

type Api = ReturnType<typeof useTerminalDrive>
const session: TerminalDriveTarget = { space: 'acme/site', session: 'abc123' }

async function mountAt(
  path: string,
  initial: TerminalDriveTarget | null = session,
): Promise<{ api: Api; router: Router; target: Ref<TerminalDriveTarget | null>; active: Ref<boolean> }> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  const target = ref<TerminalDriveTarget | null>(initial)
  const active = ref(true)
  let api!: Api
  const Host = defineComponent({
    setup() {
      api = useTerminalDrive(target, () => active.value)
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { api, router, target, active }
}

describe('useTerminalDrive', () => {
  beforeEach(() => {
    wailsEvents.handlers = []
  })

  it('shows the pane for the attached session while the route asks for it', async () => {
    const { api, router } = await mountAt('/terminal/fix-login')
    expect(api.pane.value).toBeNull()

    await router.replace('/terminal/fix-login?doc=plans%2Fx.md')
    expect(api.pane.value).toEqual(session)
    expect(api.docPath.value).toBe('plans/x.md')
  })

  // The scratch terminal has no checkout, so it has no space to show.
  it('shows nothing for a session with no space, whatever the route says', async () => {
    const { api } = await mountAt('/terminal/Scratch?doc=1', null)
    expect(api.pane.value).toBeNull()
  })

  it('keeps the pane open across a session switch, and drops the pinned path', async () => {
    const { api, router, target } = await mountAt('/terminal/fix-login?doc=plans%2Fx.md')

    target.value = { space: 'acme/api', session: 'def456' }
    await router.push('/terminal/add-search')
    await flushPromises()

    expect(router.currentRoute.value.query.doc).toBe('1')
    expect(api.pane.value).toEqual({ space: 'acme/api', session: 'def456' })
    expect(api.docPath.value).toBeNull()
  })

  // The attach mirrors the active window into the query and rebuilds it to do
  // so. That is not a close.
  it('keeps the pane and its pinned document when the window changes under it', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?doc=plans%2Fx.md')

    await router.replace({ name: 'terminal', params: { slug: 'fix-login' }, query: { window: '@2' } })
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ window: '@2', doc: 'plans/x.md' })
    expect(api.pane.value).toEqual(session)
  })

  it('closes when only the doc query is dropped', async () => {
    const { api, router } = await mountAt('/terminal/fix-login?window=%402&doc=plans%2Fx.md')

    api.syncDocQuery(false)
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ window: '@2' })
    expect(api.pane.value).toBeNull()
  })

  it('leaves a closed pane closed across a session switch', async () => {
    const { router } = await mountAt('/terminal/fix-login')

    await router.push('/terminal/add-search')
    await flushPromises()

    expect(router.currentRoute.value.query.doc).toBeUndefined()
  })

  it('opens and closes on drive:toggle for the session in view only', async () => {
    const { router, active } = await mountAt('/terminal/fix-login')

    wailsEvents.fire('drive:toggle', { session: 0, hiveSession: 'def456', path: '', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBeUndefined()

    active.value = false
    wailsEvents.fire('drive:toggle', { session: 0, hiveSession: 'abc123', path: 'plans/x.md', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBeUndefined()

    active.value = true
    wailsEvents.fire('drive:toggle', { session: 0, hiveSession: 'abc123', path: 'plans/x.md', open: true })
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBe('plans/x.md')

    wailsEvents.fire('drive:toggle', { session: 0, hiveSession: 'abc123', path: '', open: false })
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBeUndefined()
  })

  it('marks a write unseen until the pane for that session shows it', async () => {
    const { router } = await mountAt('/terminal/fix-login')
    const { isDocUnseen } = useDriveRouteFor(router)

    wailsEvents.fire('drive:updated', { session: 0, hiveSession: 'abc123' })
    await flushPromises()
    expect(isDocUnseen('abc123')).toBe(true)

    await router.replace('/terminal/fix-login?doc=1')
    await flushPromises()
    expect(isDocUnseen('abc123')).toBe(false)

    wailsEvents.fire('drive:updated', { session: 0, hiveSession: 'abc123' })
    await flushPromises()
    expect(isDocUnseen('abc123')).toBe(false)
  })
})

// The unseen set lives in a store, read here through a second caller the
// way the title bar's toggle reads it.
function useDriveRouteFor(router: Router): ReturnType<typeof useDriveRoute> {
  let api!: ReturnType<typeof useDriveRoute>
  const Host = defineComponent({
    setup() {
      api = useDriveRoute()
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  return api
}
