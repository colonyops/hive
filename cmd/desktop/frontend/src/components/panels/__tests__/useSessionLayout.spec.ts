import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent, ref, type Ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../../router'
import { useSessionLayout, type SessionDriveTarget } from '../useSessionLayout'

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
  withOpenWindow = true,
): Promise<{
  router: Router
  target: Ref<SessionDriveTarget | null>
  active: Ref<boolean>
  reveal: ReturnType<typeof vi.fn>
  open: ReturnType<typeof vi.fn>
  close: ReturnType<typeof vi.fn>
  openWindow: ReturnType<typeof vi.fn>
  move: ReturnType<typeof vi.fn>
  focus: ReturnType<typeof vi.fn>
  closePanel: ReturnType<typeof vi.fn>
  resize: ReturnType<typeof vi.fn>
  turn: ReturnType<typeof vi.fn>
}> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()

  const target = ref<SessionDriveTarget | null>(initial)
  const active = ref(true)
  const reveal = vi.fn()
  const open = vi.fn()
  const close = vi.fn()
  const openWindow = vi.fn()
  const move = vi.fn()
  const focus = vi.fn()
  const closePanel = vi.fn()
  const resize = vi.fn()
  const turn = vi.fn()
  const Host = defineComponent({
    setup() {
      useSessionLayout(routeName, target, () => active.value, {
        ready: () => true,
        reveal,
        open,
        close,
        openWindow: withOpenWindow ? openWindow : undefined,
        move,
        focus,
        closePanel,
        resize,
        turn,
      })
      return () => null
    },
  })
  mount(Host, { global: { plugins: [router] } })
  await flushPromises()
  return { router, target, active, reveal, open, close, openWindow, move, focus, closePanel, resize, turn }
}

function fireTab(payload: Record<string, unknown>) {
  wailsEvents.fire('session:layout', { session: 'abc123', verb: 'open', placement: 'behind', ...payload })
}

describe('useSessionLayout', () => {
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

    expect(open).toHaveBeenCalledWith('docs', 'a.md', 'behind', null)
    expect(reveal).not.toHaveBeenCalled()
  })

  it('opens the Drive tab beside the current one', async () => {
    const { open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'drive', path: '', placement: 'beside' })

    expect(open).toHaveBeenCalledWith('docs', null, 'beside', null)
  })

  it('treats any other placement as behind', async () => {
    const { open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'drive', placement: 'sideways' })

    expect(open).toHaveBeenCalledWith('docs', null, 'behind', null)
  })

  it('opens the Diff tab', async () => {
    const { open } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'diff', path: '' })

    expect(open).toHaveBeenCalledWith('diff', null, 'behind', null)
  })

  it('closes the tab of the view named', async () => {
    const { close } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'tasks', verb: 'close' })
    fireTab({ view: 'drive', verb: 'close' })

    expect(close).toHaveBeenNthCalledWith(1, 'tasks')
    expect(close).toHaveBeenNthCalledWith(2, 'docs')
  })

  it('hands a terminal window to the host with its placement', async () => {
    const { open, openWindow } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'terminal', window: '@12', placement: 'beside' })
    fireTab({ view: 'terminal', window: '@13' })

    expect(openWindow).toHaveBeenNthCalledWith(1, '@12', 'beside', null)
    expect(openWindow).toHaveBeenNthCalledWith(2, '@13', 'behind', null)
    expect(open).not.toHaveBeenCalled()
  })

  it('opens a tab below the current one', async () => {
    const { open, openWindow } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'diff', placement: 'below' })
    fireTab({ view: 'terminal', window: '@12', placement: 'below' })

    expect(open).toHaveBeenCalledWith('diff', null, 'below', null)
    expect(openWindow).toHaveBeenCalledWith('@12', 'below', null)
  })

  it('passes the anchor of an open, and no anchor when both halves are empty', async () => {
    const { open, openWindow } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'drive', placement: 'beside', anchorView: 'terminal', anchorWindow: '@3' })
    fireTab({ view: 'terminal', window: '@12', placement: 'beside', anchorView: 'diff' })
    fireTab({ view: 'tasks', anchorView: '', anchorWindow: '' })

    expect(open).toHaveBeenNthCalledWith(1, 'docs', null, 'beside', { view: 'terminal', window: '@3' })
    expect(openWindow).toHaveBeenCalledWith('@12', 'beside', { view: 'diff', window: '' })
    expect(open).toHaveBeenNthCalledWith(2, 'tasks', null, 'behind', null)
  })

  it('moves a tab relative to an anchor', async () => {
    const { move } = await mountAt('/terminal/fix-login')

    fireTab({ verb: 'move', view: 'drive', placement: 'beside', anchorView: 'terminal', anchorWindow: '@3' })
    fireTab({ verb: 'move', view: 'terminal', window: '@4' })

    expect(move).toHaveBeenNthCalledWith(1, { view: 'drive', window: '' }, 'beside', { view: 'terminal', window: '@3' })
    expect(move).toHaveBeenNthCalledWith(2, { view: 'terminal', window: '@4' }, 'behind', null)
  })

  it('focuses, closes the panel of, and turns the split around a tab', async () => {
    const { focus, closePanel, turn } = await mountAt('/terminal/fix-login')

    fireTab({ verb: 'focus', view: 'diff' })
    fireTab({ verb: 'close_panel', view: 'terminal', window: '@3' })
    fireTab({ verb: 'turn', view: 'tasks' })

    expect(focus).toHaveBeenCalledWith({ view: 'diff', window: '' })
    expect(closePanel).toHaveBeenCalledWith({ view: 'terminal', window: '@3' })
    expect(turn).toHaveBeenCalledWith({ view: 'tasks', window: '' })
  })

  it('resizes with the share the tab is to get', async () => {
    const { resize } = await mountAt('/terminal/fix-login')

    fireTab({ verb: 'resize', view: 'drive', share: 0.65 })

    expect(resize).toHaveBeenCalledWith({ view: 'drive', window: '' }, 0.65)
  })

  it('ignores a resize with no usable share', async () => {
    const { resize } = await mountAt('/terminal/fix-login')

    fireTab({ verb: 'resize', view: 'drive' })
    fireTab({ verb: 'resize', view: 'drive', share: '0.5' })
    fireTab({ verb: 'resize', view: 'drive', share: null })

    expect(resize).not.toHaveBeenCalled()
  })

  it('ignores a verb it does not know, and a verb with no view', async () => {
    const { open, close, move, focus, closePanel, resize, turn } = await mountAt('/terminal/fix-login')

    fireTab({ verb: 'explode', view: 'drive' })
    fireTab({ verb: 'constructor', view: 'drive' })
    fireTab({ verb: 'move', view: 'canvas' })
    fireTab({ verb: 'focus' })
    fireTab({ verb: undefined, view: 'drive' })

    for (const handler of [open, close, move, focus, closePanel, resize, turn]) expect(handler).not.toHaveBeenCalled()
  })

  it('ignores a terminal that names no window', async () => {
    const { open, close, openWindow } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'terminal', window: '' })
    fireTab({ view: 'terminal' })
    fireTab({ view: 'terminal', window: '@12', verb: 'close' })

    expect(openWindow).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
  })

  it('ignores a terminal when the host cannot place windows', async () => {
    const { open, close, openWindow } = await mountAt('/workspaces/web-app?chat=7', session, 'agents', false)

    fireTab({ view: 'terminal', window: '@12' })

    expect(openWindow).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
  })

  it('ignores a view it does not know', async () => {
    const { open, close } = await mountAt('/terminal/fix-login')

    fireTab({ view: 'canvas' })
    fireTab({ view: 'canvas', verb: 'close' })
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

    expect(open).toHaveBeenCalledWith('diff', null, 'behind', null)
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
      expect(open).toHaveBeenCalledWith('docs', 'a.md', 'behind', null)

      fireTab({ view: 'drive', verb: 'close' })
      expect(close).toHaveBeenCalledWith('docs')
    })

    it('ignores an agent in another chat', async () => {
      const { open } = await mountAt('/workspaces/web-app?chat=7', session, 'agents')

      fireTab({ view: 'drive', session: 'other' })

      expect(open).not.toHaveBeenCalled()
    })
  })
})
