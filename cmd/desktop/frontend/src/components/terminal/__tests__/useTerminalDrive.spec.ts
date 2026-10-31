import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent, ref, type Ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../../router'
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

const session: TerminalDriveTarget = { space: 'acme/site', session: 'abc123' }

async function mountAt(
  path: string,
  initial: TerminalDriveTarget | null = session,
): Promise<{
  router: Router
  target: Ref<TerminalDriveTarget | null>
  active: Ref<boolean>
  open: ReturnType<typeof vi.fn>
  arrive: ReturnType<typeof vi.fn>
  close: ReturnType<typeof vi.fn>
}> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  const target = ref<TerminalDriveTarget | null>(initial)
  const active = ref(true)
  const open = vi.fn()
  const arrive = vi.fn()
  const close = vi.fn()
  const Host = defineComponent({
    setup() {
      useTerminalDrive(target, () => active.value, { ready: () => true, open, arrive, close })
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { router, target, active, open, arrive, close }
}

describe('useTerminalDrive', () => {
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
})
