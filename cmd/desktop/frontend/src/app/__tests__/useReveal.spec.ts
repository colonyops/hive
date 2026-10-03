import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import type { FlowsSession } from '../../pipeline/composables/useFlowsSession'
import type { AppNavigation } from '../useAppNavigation'

const mocks = vi.hoisted(() => ({
  Feed: vi.fn(),
  FindItems: vi.fn(),
  handlers: new Map<string, (event: { data: unknown }) => void>(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice', () => ({
  Feed: mocks.Feed,
  FindItems: mocks.FindItems,
}))
vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, handler: (event: { data: unknown }) => void) => {
      mocks.handlers.set(name, handler)
      return () => mocks.handlers.delete(name)
    },
  },
}))

import { useReveal } from '../useReveal'

const push = vi.fn()
const nav = {
  router: { push },
  openFeed: vi.fn(),
  openFlows: vi.fn(),
  selectApplicationSettingsSection: vi.fn(),
}
const session = {
  activeFlow: ref({ nodes: [{ id: 'a' }, { id: 'b' }, { id: 'c' }] }),
  latestRunByNode: ref(
    new Map([
      ['a', { ok: true }],
      ['b', { ok: false }],
      ['c', { ok: false }],
    ]),
  ),
}
const showToast = vi.fn()

function setup() {
  const scope = effectScope()
  const reveal = scope.run(() =>
    useReveal(nav as unknown as AppNavigation, session as unknown as FlowsSession, showToast),
  )!
  return reveal
}

const emit = (name: string, data: unknown) => mocks.handlers.get(name)?.({ data })

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Feed.mockResolvedValue('inbox')
})

describe('useReveal', () => {
  it('counts failed nodes and opens the first one', () => {
    const reveal = setup()
    expect(reveal.errorCount.value).toBe(2)
    reveal.openErrorNode()
    expect(nav.openFlows).toHaveBeenCalledWith('b')
  })

  it('routes a clicked notification to its item in the feed that holds it', async () => {
    setup()
    emit('notification:activated', [{ profileId: 'work', itemId: 7 }])
    await flushPromises()
    expect(push).toHaveBeenCalledWith({
      name: 'feed',
      params: { profileId: 'work' },
      query: { item: '7', feed: 'inbox' },
    })
  })

  it('lands on the profile when the notification names no item, and on Trash when no feed holds it', async () => {
    setup()
    emit('notification:activated', { profileId: 'work', itemId: 0 })
    expect(nav.openFeed).toHaveBeenCalledWith('work')

    mocks.Feed.mockResolvedValue('')
    emit('menubar:open', { profileId: 'work', itemId: 3, feedId: '', settings: false })
    await flushPromises()
    expect(push).toHaveBeenCalledWith({
      name: 'feed',
      params: { profileId: 'work' },
      query: { item: '3', view: 'trash' },
    })
  })

  it('opens menu bar settings and feeds', async () => {
    setup()
    emit('menubar:open', { settings: true })
    expect(nav.selectApplicationSettingsSection).toHaveBeenCalledWith('menubar')

    emit('menubar:open', { profileId: 'work', itemId: 0, feedId: 'prs', settings: false })
    await flushPromises()
    expect(push).toHaveBeenCalledWith({ name: 'feed', params: { profileId: 'work' }, query: { feed: 'prs' } })
  })

  it('shows an in-app notification, reading an unknown severity as info', () => {
    setup()
    emit('notification:toast', { title: 'Build failed', body: 'main', severity: 'critical' })
    expect(showToast).toHaveBeenCalledWith('Build failed', { body: 'main', severity: 'info' })
  })

  it('reveals an Activity row by its source, and says so when nothing matches', async () => {
    const reveal = setup()
    const link = { profileId: 'work', externalId: 'pr-1', sourceKind: 'github', sourceScope: 'acme/site' }
    mocks.FindItems.mockResolvedValue([
      { id: 1, sourceKind: 'github', sourceScope: 'other/repo' },
      { id: 2, sourceKind: 'github', sourceScope: 'acme/site' },
    ])
    await reveal.openActivityItem(link)
    expect(push).toHaveBeenCalledWith(expect.objectContaining({ query: { item: '2', feed: 'inbox' } }))

    mocks.FindItems.mockResolvedValue([])
    await reveal.openActivityItem(link)
    expect(showToast).toHaveBeenCalledWith('Could not find the linked item', { severity: 'error' })
  })
})
