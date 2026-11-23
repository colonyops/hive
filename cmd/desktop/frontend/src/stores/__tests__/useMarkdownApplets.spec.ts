import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fenceRenderer } from '@hive/ui/components/fences/registry'
import { commands } from '../../keybindings/catalog'
import { resetStores } from '../defineStore'
import { useApplets } from '../useApplets'
import { useMarkdownApplets } from '../useMarkdownApplets'

const service = vi.hoisted(() => ({ List: vi.fn(), SetEnabled: vi.fn(), Delete: vi.fn(), SetSurface: vi.fn() }))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice',
  () => service,
)

const applet = (id: string, markdown: object | null, extra: object = {}) => ({
  id,
  title: id,
  entry: 'index.html',
  icon: '',
  runnable: true,
  enabled: true,
  surfaces: { tab: false, page: false, popup: null, titlebar: null, markdown },
  ...extra,
})
const markdown = (fences: string[], extra: object = {}) => ({
  entry: 'index.html',
  fences,
  body: 'yaml',
  enabled: true,
  ...extra,
})

async function load(list: object[]) {
  service.List.mockResolvedValue(list)
  const applets = useApplets()
  useMarkdownApplets()
  await flushPromises()
  return applets
}

describe('useMarkdownApplets', () => {
  beforeEach(() => {
    resetStores()
  })

  it('registers each fence of a runnable, enabled applet with its body type', async () => {
    await load([applet('cards', markdown(['pr-card'])), applet('notes', markdown(['note'], { body: 'text' }))])

    expect(fenceRenderer('pr-card')).toMatchObject({ builtin: false, body: 'yaml' })
    expect(fenceRenderer('note')).toMatchObject({ builtin: false, body: 'text' })
  })

  it('unregisters when the applet is turned off, loses the surface, or disappears', async () => {
    service.List.mockResolvedValue([
      applet('a', markdown(['fence-a'])),
      applet('b', markdown(['fence-b'])),
      applet('c', markdown(['fence-c'])),
    ])
    const applets = await load([
      applet('a', markdown(['fence-a'])),
      applet('b', markdown(['fence-b'])),
      applet('c', markdown(['fence-c'])),
    ])
    expect(fenceRenderer('fence-a')).toBeDefined()

    service.List.mockResolvedValue([
      applet('a', markdown(['fence-a']), { enabled: false }),
      applet('b', markdown(['fence-b'], { enabled: false })),
    ])
    await applets.reload()

    expect(fenceRenderer('fence-a')).toBeUndefined()
    expect(fenceRenderer('fence-b')).toBeUndefined()
    expect(fenceRenderer('fence-c')).toBeUndefined()
  })

  it('swaps a fence when the applet changes its list', async () => {
    const applets = await load([applet('cards', markdown(['old-card']))])

    service.List.mockResolvedValue([applet('cards', markdown(['new-card']))])
    await applets.reload()

    expect(fenceRenderer('old-card')).toBeUndefined()
    expect(fenceRenderer('new-card')).toBeDefined()
  })

  it('leaves a fence without a runnable applet unregistered', async () => {
    await load([applet('broken', markdown(['broken-card']), { runnable: false })])

    expect(fenceRenderer('broken-card')).toBeUndefined()
  })

  it('gives a markdown-only applet no open row but keeps its declared commands', async () => {
    await load([
      applet('cards', markdown(['pr-card']), { commands: [{ id: 'refresh', title: 'Refresh cards' }] }),
      applet('quiet', markdown(['quiet-card'])),
    ])

    const ids = commands.value.map((command) => command.id)
    expect(ids).toContain('applet.cards.refresh')
    expect(ids).not.toContain('applet.cards')
    expect(ids).not.toContain('applet.quiet')
  })
})
