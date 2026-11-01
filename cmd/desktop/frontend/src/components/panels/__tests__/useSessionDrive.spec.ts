import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent, ref, type Ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../../router'
import { useSessionDrive, type SessionDriveTarget } from '../useSessionDrive'

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

const session: SessionDriveTarget = { space: 'acme/site', session: 'abc123' }

async function mountAt(
  path: string,
  initial: SessionDriveTarget | null = session,
  routeName: 'terminal' | 'agents' = 'terminal',
): Promise<{
  router: Router
  target: Ref<SessionDriveTarget | null>
  active: Ref<boolean>
  open: ReturnType<typeof vi.fn>
  arrive: ReturnType<typeof vi.fn>
  close: ReturnType<typeof vi.fn>
}> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  const target = ref<SessionDriveTarget | null>(initial)
  const active = ref(true)
  const open = vi.fn()
  const arrive = vi.fn()
  const close = vi.fn()
  const Host = defineComponent({
    setup() {
      useSessionDrive(routeName, target, () => active.value, { ready: () => true, open, arrive, close })
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { router, target, active, open, arrive, close }
}

describe('useSessionDrive', () => {
  beforeEach(() => {
    wailsEvents.handlers = []
  })

  it('opens the tab on the route document and consumes the query', async () => {
    const { router, open } = await mountAt('/terminal/fix-login')
    expect(open).not.toHaveBeenCalled()

    await router.replace('/terminal/fix-login?doc=plans%2Fx.md')
    await flushPromises()

    expect(open).toHaveBeenCalledWith('plans/x.md')
    expect(router.currentRoute.value.query.doc).toBeUndefined()
  })

  it('waits for a session that has a space', async () => {
    const { router, target, open } = await mountAt('/terminal/fix-login?doc=1', null)
    expect(open).not.toHaveBeenCalled()

    target.value = session
    await flushPromises()

    expect(open).toHaveBeenCalledWith(null)
    expect(router.currentRoute.value.query.doc).toBeUndefined()
  })

  it('follows an agent on the session in view', async () => {
    const { arrive, close } = await mountAt('/terminal/fix-login')

    wailsEvents.fire('drive:toggle', { open: true, path: 'a.md', session: 'abc123' })
    expect(arrive).toHaveBeenCalledWith('a.md')

    wailsEvents.fire('drive:toggle', { open: false, session: 'abc123' })
    expect(close).toHaveBeenCalled()
  })

  it('ignores an agent in another session, and any agent off-screen', async () => {
    const { active, arrive } = await mountAt('/terminal/fix-login')

    wailsEvents.fire('drive:toggle', { open: true, session: 'other' })
    active.value = false
    wailsEvents.fire('drive:toggle', { open: true, session: 'abc123' })

    expect(arrive).not.toHaveBeenCalled()
  })

  it("does nothing on the other area's route", async () => {
    const { router, open, arrive } = await mountAt('/workspaces/web-app?chat=7&doc=1', session, 'terminal')
    wailsEvents.fire('drive:toggle', { open: true, session: 'abc123' })

    expect(open).not.toHaveBeenCalled()
    expect(arrive).not.toHaveBeenCalled()
    expect(router.currentRoute.value.query.doc).toBe('1')
  })

  describe('on the agents route', () => {
    it('opens the tab on the route document and consumes the query', async () => {
      const { router, open } = await mountAt('/workspaces/web-app?chat=7&doc=plans%2Fx.md', session, 'agents')

      expect(open).toHaveBeenCalledWith('plans/x.md')
      expect(router.currentRoute.value.query.doc).toBeUndefined()
      expect(router.currentRoute.value.query.chat).toBe('7')
    })

    it('follows an agent on the chat in view', async () => {
      const { arrive, close } = await mountAt('/workspaces/web-app?chat=7', session, 'agents')

      wailsEvents.fire('drive:toggle', { open: true, path: 'a.md', session: 'abc123' })
      expect(arrive).toHaveBeenCalledWith('a.md')

      wailsEvents.fire('drive:toggle', { open: false, session: 'abc123' })
      expect(close).toHaveBeenCalled()
    })

    it('ignores an agent in another chat', async () => {
      const { arrive } = await mountAt('/workspaces/web-app?chat=7', session, 'agents')

      wailsEvents.fire('drive:toggle', { open: true, session: 'other' })

      expect(arrive).not.toHaveBeenCalled()
    })
  })
})
