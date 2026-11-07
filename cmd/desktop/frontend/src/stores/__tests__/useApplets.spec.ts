import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { appletTab } from '../../lib/appletTabs'
import { openTab, panelWithTab } from '../../lib/panelLayout'
import type { AppletSurface } from '../appletSurfaces'
import { resetStores } from '../defineStore'
import { useApplets } from '../useApplets'
import { useChatPanelLayouts, usePanelLayouts } from '../usePanelLayouts'

const service = vi.hoisted(() => ({ List: vi.fn(), SetEnabled: vi.fn(), Delete: vi.fn() }))
const toasts = vi.hoisted(() => ({ showToast: vi.fn() }))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice',
  () => service,
)
vi.mock('../../composables/useToasts', () => ({ useToasts: () => toasts }))

const applet = (id: string, extra: object = {}) => ({
  id,
  title: id,
  entry: 'index.html',
  icon: '',
  runnable: true,
  enabled: true,
  surfaces: { tab: false, popup: {}, titlebar: null },
  ...extra,
})
const tabOnly = { surfaces: { tab: true, popup: null, titlebar: null } }
const pageOnly = { surfaces: { tab: false, page: true, popup: null, titlebar: null } }
const tabAndPopup = { surfaces: { tab: true, popup: {}, titlebar: null } }
const titlebarOnly = { surfaces: { tab: false, popup: null, titlebar: { entry: 'bar.html', width: 28 } } }

function hostOf(slug = '') {
  return { open: vi.fn(), sessionSlug: () => slug }
}

async function loadedStore() {
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
    const applets = await loadedStore()
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
    const applets = await loadedStore()
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
    const applets = await loadedStore()
    const hidden = frame('tab', false)
    const shown = frame('popup')
    applets.connectFrame('radio', hidden)
    applets.connectFrame('radio', shown)
    expect(applets.deliverCommand('radio', 'next')).toBe(true)
    expect(shown.deliverCommand).toHaveBeenCalledWith('next')
    expect(hidden.deliverCommand).not.toHaveBeenCalled()
  })

  it('says no frame took a command when the only tab is out of sight', async () => {
    const applets = await loadedStore()
    const hidden = frame('tab', false)
    applets.connectFrame('radio', hidden)
    expect(applets.deliverCommand('radio', 'next')).toBe(false)
    expect(hidden.deliverCommand).not.toHaveBeenCalled()
  })

  it('holds a command for a tab that comes forward to take it', async () => {
    service.List.mockResolvedValue([applet('radio', tabOnly)])
    const applets = await loadedStore()
    const host = hostOf()
    applets.setTabHost(host, true)
    let onScreen = false
    const tab = { ...frame('tab'), onScreen: () => onScreen }
    applets.connectFrame('radio', tab)

    applets.openForCommand('radio', 'next')
    expect(host.open).toHaveBeenCalledWith('radio')
    expect(tab.deliverCommand).not.toHaveBeenCalled()

    onScreen = true
    applets.frameShown('radio', tab)
    expect(tab.deliverCommand).toHaveBeenCalledWith('next')
  })

  it('offers a palette row only from a frame on screen', async () => {
    const applets = await loadedStore()
    const hidden = frame('tab', false)
    applets.setFrameRows(hidden, 'radio', [{ id: 'next', title: 'Next' }])
    expect(applets.pageRows.value).toEqual([])

    const shown = frame('popup')
    applets.setFrameRows(shown, 'radio', [{ id: 'next', title: 'Next' }])
    void applets.pageRows.value[0]?.run()
    expect(shown.deliverCommand).toHaveBeenCalledWith('next')
    expect(hidden.deliverCommand).not.toHaveBeenCalled()
  })
})

describe('useApplets surfaces', () => {
  beforeEach(() => {
    resetStores()
    toasts.showToast.mockClear()
    service.List.mockResolvedValue([applet('radio', tabAndPopup)])
  })

  it('lists what its file puts in the + menu and the title bar, among those that can run', async () => {
    service.List.mockResolvedValue([
      applet('radio', tabOnly),
      applet('pop'),
      applet('off', { ...tabOnly, enabled: false }),
      applet('bad', { ...tabOnly, runnable: false }),
      applet('bar', titlebarOnly),
      applet('off-bar', { ...titlebarOnly, enabled: false }),
    ])
    const applets = await loadedStore()
    expect(applets.sessionMenu.value.map((row) => row.id)).toEqual(['radio'])
    expect(applets.titlebarApplets.value.map((row) => row.id)).toEqual(['bar'])
  })

  it('lists only applets whose file names a page as global tabs, never in the session menu', async () => {
    service.List.mockResolvedValue([
      applet('board', pageOnly),
      applet('radio', tabOnly),
      applet('pop'),
      applet('off-board', { ...pageOnly, enabled: false }),
    ])
    const applets = await loadedStore()
    expect(applets.globalTabApplets.value.map((row) => row.id)).toEqual(['board'])
    expect(applets.sessionMenu.value.map((row) => row.id)).toEqual(['radio'])
  })

  it('opens a page as a global tab, adding it to the strip, even with a session on screen', async () => {
    service.List.mockResolvedValue([applet('board', pageOnly)])
    const applets = await loadedStore()
    const host = hostOf()
    applets.setTabHost(host, true)
    const opener = vi.fn((_id: string, add: boolean) => add)
    applets.setPageTabOpener(opener)
    applets.toggle('board')
    expect(host.open).not.toHaveBeenCalled()
    expect(opener).toHaveBeenLastCalledWith('board', true)
  })

  it('prefers the session tab for an applet with both, unless its global tab is already in the strip', async () => {
    service.List.mockResolvedValue([
      applet('both', { surfaces: { tab: true, page: true, popup: null, titlebar: null } }),
    ])
    const applets = await loadedStore()
    const host = hostOf()
    applets.setTabHost(host, true)
    let inStrip = false
    applets.setPageTabOpener((_id, add) => add || inStrip)
    applets.toggle('both')
    expect(host.open).toHaveBeenCalledWith('both')

    host.open.mockClear()
    inStrip = true
    applets.toggle('both')
    expect(host.open).not.toHaveBeenCalled()
  })

  it('opens an applet with a tab as a tab where a session area can show one', async () => {
    const applets = await loadedStore()
    const host = hostOf()
    applets.setTabHost(host, true)
    applets.toggle('radio')
    expect(host.open).toHaveBeenCalledWith('radio')
    expect(applets.open.value).toBeNull()
  })

  it('opens the pop-up the file also lists with no session area on screen', async () => {
    const applets = await loadedStore()
    const host = hostOf()
    applets.setTabHost(host, true)
    applets.setTabHost(host, false)
    applets.toggle('radio')
    expect(host.open).not.toHaveBeenCalled()
    expect(applets.open.value?.id).toBe('radio')
  })

  it('never opens a pop-up the file does not list, and says why', async () => {
    service.List.mockResolvedValue([applet('radio', tabOnly)])
    const applets = await loadedStore()
    applets.toggle('radio')
    expect(applets.open.value).toBeNull()
    expect(toasts.showToast).toHaveBeenCalledWith(expect.stringContaining('Open a session'), expect.anything())
  })

  it('still opens a broken file, to show its error', async () => {
    service.List.mockResolvedValue([
      applet('bad', { runnable: false, surfaces: { tab: false, popup: null, titlebar: null } }),
    ])
    const applets = await loadedStore()
    applets.toggle('bad')
    expect(applets.open.value?.id).toBe('bad')
  })

  it('drops a command when the applet has nowhere to open', async () => {
    service.List.mockResolvedValue([applet('radio', tabOnly)])
    const applets = await loadedStore()
    applets.openForCommand('radio', 'next')
    const host = hostOf()
    applets.setTabHost(host, true)
    const tab = { surface: 'tab' as const, post: vi.fn(), deliverCommand: vi.fn(), onScreen: () => true }
    applets.connectFrame('radio', tab)
    expect(tab.deliverCommand).not.toHaveBeenCalled()
  })

  it('closes the pop-up when an edit stops listing it', async () => {
    const applets = await loadedStore()
    applets.toggle('radio')
    expect(applets.open.value?.id).toBe('radio')
    service.List.mockResolvedValue([applet('radio', tabOnly)])
    await applets.reload()
    expect(applets.open.value).toBeNull()
  })

  it('gives a pop-up the session of the area on screen, in Chats as in Code', async () => {
    service.List.mockResolvedValue([applet('radio')])
    const applets = await loadedStore()
    applets.setTabHost(hostOf('chat-slug'), true)
    expect(applets.liveSessionSlug.value).toBe('chat-slug')
    applets.toggle('radio')
    expect(applets.sessionSlug.value).toBe('chat-slug')
  })

  it('uses the session the route names until an area can answer, as during a cold attach', async () => {
    service.List.mockResolvedValue([applet('radio')])
    const applets = await loadedStore()
    applets.setRouteSession('from-route')
    applets.toggle('radio')
    expect(applets.sessionSlug.value).toBe('from-route')
    applets.setTabHost(hostOf('from-area'), true)
    expect(applets.liveSessionSlug.value).toBe('from-area')
  })

  it('keeps the area on screen as the host when another one lets go', async () => {
    const applets = await loadedStore()
    const code = hostOf()
    applets.setTabHost(code, true)
    applets.setTabHost(hostOf(), false)
    applets.toggle('radio')
    expect(code.open).toHaveBeenCalledWith('radio')
  })

  it('closes another pop-up that would cover the tab it opens', async () => {
    service.List.mockResolvedValue([applet('radio', tabAndPopup), applet('other')])
    const applets = await loadedStore()
    applets.setTabHost(hostOf(), true)
    applets.toggle('other')
    applets.toggle('radio')
    expect(applets.open.value).toBeNull()
  })

  it('closes a dropdown when an edit replaces the frame it hangs from', async () => {
    service.List.mockResolvedValue([applet('radio', { revision: 1 })])
    const applets = await loadedStore()
    const opened = applets.applets.value[0] as never
    applets.toggleDropdown({ applet: opened, entry: 'm.html', width: 1, height: 1, anchor: document.body })
    expect(applets.dropdown.value).not.toBeNull()

    service.List.mockResolvedValue([applet('radio', { revision: 2 })])
    await applets.reload()
    expect(applets.dropdown.value).toBeNull()
  })
})
