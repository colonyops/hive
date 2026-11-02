import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { appletTab } from '../../lib/appletTabs'
import { openTab, panelWithTab } from '../../lib/panelLayout'
import type { AppletSurface } from '../appletSurfaces'
import { resetStores } from '../defineStore'
import { useApplets } from '../useApplets'
import { useChatPanelLayouts, usePanelLayouts } from '../usePanelLayouts'

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

describe('useApplets delete', () => {
  beforeEach(() => {
    resetStores()
    service.List.mockResolvedValue([applet('radio')])
    service.Delete.mockResolvedValue(undefined)
  })

  it('closes the applet tab in every saved layout, and only that tab', async () => {
    const applets = await storeWith({})
    const code = usePanelLayouts()
    const chats = useChatPanelLayouts()
    code.apply('off-screen', (current) => openTab(openTab(current, appletTab('radio')), appletTab('other')))
    chats.apply('chat', (current) => openTab(current, appletTab('radio')))

    await applets.remove('radio')

    expect(panelWithTab(code.layoutOf('off-screen'), 'applet:radio')).toBeUndefined()
    expect(panelWithTab(code.layoutOf('off-screen'), 'applet:other')).toBeDefined()
    expect(panelWithTab(chats.layoutOf('chat'), 'applet:radio')).toBeUndefined()
  })

  it('keeps the tab when the delete fails', async () => {
    service.Delete.mockRejectedValue(new Error('no'))
    const applets = await storeWith({})
    const code = usePanelLayouts()
    code.apply('s', (current) => openTab(current, appletTab('radio')))
    await expect(applets.remove('radio')).rejects.toThrow('no')
    expect(panelWithTab(code.layoutOf('s'), 'applet:radio')).toBeDefined()
  })
})

describe('useApplets commands', () => {
  const frame = (surface: AppletSurface, onScreen = true) => ({
    surface,
    post: vi.fn(),
    deliverCommand: vi.fn(),
    onScreen: () => onScreen,
  })

  beforeEach(() => {
    resetStores()
    service.List.mockResolvedValue([applet('radio')])
  })

  it('gives a command to the frame on screen, not to a tab the user cannot see', async () => {
    const applets = await storeWith({})
    const hidden = frame('tab', false)
    const shown = frame('page')
    applets.connectFrame('radio', hidden)
    applets.connectFrame('radio', shown)
    expect(applets.deliverCommand('radio', 'next')).toBe(true)
    expect(shown.deliverCommand).toHaveBeenCalledWith('next')
    expect(hidden.deliverCommand).not.toHaveBeenCalled()
  })

  it('says no frame took a command when the only tab is out of sight', async () => {
    const applets = await storeWith({})
    const hidden = frame('tab', false)
    applets.connectFrame('radio', hidden)
    expect(applets.deliverCommand('radio', 'next')).toBe(false)
    expect(hidden.deliverCommand).not.toHaveBeenCalled()
  })

  it('still prefers a running title bar page', async () => {
    const applets = await storeWith({})
    const bar = frame('titlebar')
    applets.connectFrame('radio', frame('tab'))
    applets.connectFrame('radio', bar)
    applets.deliverCommand('radio', 'next')
    expect(bar.deliverCommand).toHaveBeenCalledWith('next')
  })

  it('holds a command for a tab that comes forward to take it', async () => {
    const applets = await storeWith({ tab: ['radio'] })
    const host = vi.fn()
    applets.setTabHost(host, true)
    let onScreen = false
    const tab = { ...frame('tab'), onScreen: () => onScreen }
    applets.connectFrame('radio', tab)

    applets.openForCommand('radio', 'next', 'review')
    expect(host).toHaveBeenCalledWith('radio')
    expect(tab.deliverCommand).not.toHaveBeenCalled()

    onScreen = true
    applets.frameShown('radio', tab)
    expect(tab.deliverCommand).toHaveBeenCalledWith('next')
  })

  it('runs a palette row in the frame on screen when several offer it', async () => {
    const applets = await storeWith({})
    const hidden = frame('tab', false)
    const shown = frame('page')
    for (const handle of [shown, hidden]) applets.setFrameRows(handle, 'radio', [{ id: 'next', title: 'Next' }])
    void applets.pageRows.value[0]?.run()
    expect(shown.deliverCommand).toHaveBeenCalledWith('next')
    expect(hidden.deliverCommand).not.toHaveBeenCalled()
  })
})

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
