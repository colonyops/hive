import { createMemoryHistory, type Router } from 'vue-router'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../router'
import { useDriveRoute } from '../useDriveRoute'

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn().mockReturnValue(() => {}) } }))

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
    api.syncDocQuery(false)
    await flushPromises()

    await router.replace({ query: { chat: '7', doc: 'plans/x.md' } })
    await flushPromises()
    api.syncDocQuery(true)
    await flushPromises()
    expect(router.currentRoute.value.query.doc).toBe('plans/x.md')
  })

  // The dot is keyed by the authoring chat: a write to the chat in view with
  // its pane open is already read, a write to any other chat is remembered and
  // lights when that chat comes into view.
  it('keys the unseen dot by chat', async () => {
    const { api, router } = await mountAt('/workspaces/web-app?chat=7')
    expect(api.docUnseen.value).toBe(false)

    api.noteDriveWrite('9')
    expect(api.docUnseen.value).toBe(false)

    api.noteDriveWrite('7')
    expect(api.docUnseen.value).toBe(true)

    api.clearDocUnseen('7')
    expect(api.docUnseen.value).toBe(false)

    // The write to 9 was remembered, so switching to it lights its own dot.
    await router.replace({ query: { chat: '9' } })
    await flushPromises()
    expect(api.docUnseen.value).toBe(true)
  })

  it('ignores a write while that chat has its pane on screen', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7&doc=1')
    api.noteDriveWrite('7')
    expect(api.docUnseen.value).toBe(false)
  })

  it('ignores a write with no usable session id', async () => {
    const { api } = await mountAt('/workspaces/web-app?chat=7')
    api.noteDriveWrite('')
    expect(api.docUnseen.value).toBe(false)
  })

  // The set lives in a store so the title bar and AgentsMode read one truth.
  it('shares the unseen set across callers', async () => {
    const { api: first, router } = await mountAt('/workspaces/web-app?chat=7')
    first.noteDriveWrite('7')

    let second!: Api
    const Host = defineComponent({
      setup() {
        second = useDriveRoute()
        return () => null
      },
    })
    mount(Host, { global: { plugins: [router] } })
    await flushPromises()

    expect(second.docUnseen.value).toBe(true)
    second.clearDocUnseen('7')
    expect(first.docUnseen.value).toBe(false)
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

  it('keeps an unseen mark per author, chat or hive session', async () => {
    const { api } = await mountAt('/terminal/fix-login')

    api.markDocUnseen('abc123')
    expect(api.isDocUnseen('abc123')).toBe(true)
    expect(api.isDocUnseen('def456')).toBe(false)
    expect(api.isDocUnseen(null)).toBe(false)

    api.clearDocUnseen('abc123')
    expect(api.isDocUnseen('abc123')).toBe(false)
  })
})
