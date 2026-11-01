import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent, ref, type Ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../../router'
import { useSessionTabs, type SessionDriveTarget } from '../useSessionTabs'

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
  reveal: ReturnType<typeof vi.fn>
  open: ReturnType<typeof vi.fn>
  close: ReturnType<typeof vi.fn>
}> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  const target = ref<SessionDriveTarget | null>(initial)
  const active = ref(true)
  const reveal = vi.fn()
  const open = vi.fn()
  const close = vi.fn()
  const Host = defineComponent({
    setup() {
      useSessionTabs(routeName, target, () => active.value, { ready: () => true, reveal, open, close })
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { router, target, active, reveal, open, close }
}

function fireTab(payload: Record<string, unknown>) {
  wailsEvents.fire('session:tab', { session: 'abc123', placement: 'behind', open: true, ...payload })
}

describe('useSessionTabs', () => {
  beforeEach(() => {
    wailsEvents.handlers = []
  })

  it('reveals the route document and consumes the query', async () => {
    const { router, reveal } = await mountAt('/terminal/fix-login')
    expect(reveal).not.toHaveBeenCalled()

    await router.replace('/terminal/fix-login?doc=plans%2Fx.md')
    await flushPromises()

    expect(reveal).toHaveBeenCalledWith('plans/x.md')
    expect(router.currentRoute.value.query.doc).toBeUndefined()
  })

  it('waits for a session that has a space', async () => {
    const { router, target, reveal } = await mountAt('/terminal/fix-login?doc=1', null)
    expect(reveal).not.toHaveBeenCalled()

    target.value = session
    await flushPromises()

    expect(reveal).toHaveBeenCalledWith(null)
    expect(router.currentRoute.value.query.doc).toBeUndefined()
  })

  it('opens the Drive tab behind the current one for an agent', async () => {
    const { open, reveal } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'drive', path: 'a.md' })

    expect(open).toHaveBeenCalledWith('docs', 'a.md', 'behind')
    expect(reveal).not.toHaveBeenCalled()
  })

  it('opens the Drive tab beside the current one', async () => {
    const { open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'drive', path: '', placement: 'beside' })

    expect(open).toHaveBeenCalledWith('docs', null, 'beside')
  })

  it('treats any other placement as behind', async () => {
    const { open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'drive', placement: 'sideways' })

    expect(open).toHaveBeenCalledWith('docs', null, 'behind')
  })

  it('opens the Diff tab', async () => {
    const { open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'diff', path: '' })

    expect(open).toHaveBeenCalledWith('diff', null, 'behind')
  })

  it('closes the tab of the view named', async () => {
    const { close } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'tasks', open: false })
    fireTab({ view: 'drive', open: false })

    expect(close).toHaveBeenNthCalledWith(1, 'tasks')
    expect(close).toHaveBeenNthCalledWith(2, 'docs')
  })

  it('ignores a view it does not know', async () => {
    const { open, close } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'canvas' })
    fireTab({ view: 'canvas', open: false })
    fireTab({ view: 'constructor' })
    fireTab({})

    expect(open).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
  })

  it('ignores an agent in another session, and any agent off-screen', async () => {
    const { active, open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'drive', session: 'other' })
    active.value = false
    fireTab({ view: 'drive' })

    expect(open).not.toHaveBeenCalled()
  })

  it('names a hive session by its hiveSession', async () => {
    const { open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'diff', session: '', hiveSession: 'abc123' })

    expect(open).toHaveBeenCalledWith('diff', null, 'behind')
  })

  it("does nothing on the other area's route", async () => {
    const { router, reveal, open } = await mountAt('/workspaces/web-app?chat=7&doc=1', session, 'terminal')
    fireTab({ view: 'drive' })

    expect(reveal).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
    expect(router.currentRoute.value.query.doc).toBe('1')
  })

  describe('on the agents route', () => {
    it('reveals the route document and consumes the query', async () => {
      const { router, reveal } = await mountAt('/workspaces/web-app?chat=7&doc=plans%2Fx.md', session, 'agents')

      expect(reveal).toHaveBeenCalledWith('plans/x.md')
      expect(router.currentRoute.value.query.doc).toBeUndefined()
      expect(router.currentRoute.value.query.chat).toBe('7')
    })

    it('follows an agent on the chat in view', async () => {
      const { open, close } = await mountAt('/workspaces/web-app?chat=7', session, 'agents')

      fireTab({ view: 'drive', path: 'a.md' })
      expect(open).toHaveBeenCalledWith('docs', 'a.md', 'behind')

      fireTab({ view: 'drive', open: false })
      expect(close).toHaveBeenCalledWith('docs')
    })

    it('ignores an agent in another chat', async () => {
      const { open } = await mountAt('/workspaces/web-app?chat=7', session, 'agents')

      fireTab({ view: 'drive', session: 'other' })

      expect(open).not.toHaveBeenCalled()
    })
  })
})
