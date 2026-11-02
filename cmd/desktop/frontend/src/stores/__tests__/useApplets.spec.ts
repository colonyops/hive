import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../defineStore'
import { useApplets } from '../useApplets'

const service = vi.hoisted(() => ({ List: vi.fn(), SetEnabled: vi.fn(), Delete: vi.fn() }))
const settings = vi.hoisted(() => ({ AppletSurface: vi.fn(), SetAppletSurface: vi.fn() }))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice',
  () => service,
)
vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settings,
)

const applet = (id: string, extra: object = {}) => ({
  id,
  title: id,
  entry: 'index.html',
  icon: '',
  runnable: true,
  enabled: true,
  ...extra,
})

async function storeWith(placed: Record<string, string[]>) {
  settings.AppletSurface.mockImplementation((surface: string) => Promise.resolve(placed[surface] ?? []))
  const applets = useApplets()
  await flushPromises()
  return applets
}

describe('useApplets placements', () => {
  beforeEach(() => {
    resetStores()
    service.List.mockResolvedValue([
      applet('radio'),
      applet('off', { enabled: false }),
      applet('bad', { runnable: false }),
    ])
  })

  it('lists what the user put in the + menu, one row per way it opens', async () => {
    const applets = await storeWith({ tab: ['radio', 'off', 'bad', 'gone'], page: ['radio'] })
    expect(applets.sessionMenu.value.map((row) => [row.applet.id, row.opensAs])).toEqual([
      ['radio', 'tab'],
      ['radio', 'page'],
    ])
  })

  it('leaves an applet out of the + menu until the user adds it', async () => {
    const applets = await storeWith({})
    expect(applets.sessionMenu.value).toEqual([])
  })

  it('opens a placed applet as a tab where a session area can show one', async () => {
    const applets = await storeWith({ tab: ['radio'] })
    const host = vi.fn()
    applets.setTabHost(host, true)
    applets.toggle('radio', 'review')
    expect(host).toHaveBeenCalledWith('radio')
    expect(applets.open.value).toBeNull()
  })

  it('opens a pop-up with no session area on screen, or for an applet not placed as a tab', async () => {
    const applets = await storeWith({ tab: ['radio'] })
    applets.toggle('radio', 'review')
    expect(applets.open.value?.id).toBe('radio')
    applets.close()

    const host = vi.fn()
    applets.setTabHost(host, true)
    applets.setTabHost(host, false)
    applets.toggle('radio', 'review')
    expect(host).not.toHaveBeenCalled()
    expect(applets.sessionSlug.value).toBe('review')
  })

  it('keeps the area on screen as the host when another one lets go', async () => {
    const applets = await storeWith({ tab: ['radio'] })
    const code = vi.fn()
    applets.setTabHost(code, true)
    applets.setTabHost(vi.fn(), false)
    applets.toggle('radio', '')
    expect(code).toHaveBeenCalledWith('radio')
  })

  it('opens the pop-up the + menu asks for even when the applet is also a tab', async () => {
    const applets = await storeWith({ tab: ['radio'], page: ['radio'] })
    const host = vi.fn()
    applets.setTabHost(host, true)
    applets.openPopup('radio', 'review')
    expect(host).not.toHaveBeenCalled()
    expect(applets.open.value?.id).toBe('radio')
  })
})
