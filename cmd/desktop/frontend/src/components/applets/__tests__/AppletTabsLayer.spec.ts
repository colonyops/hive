import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { appletTab } from '../../../lib/appletTabs'
import { newPanelLayout, openTab, selectTab, type PanelLayout } from '../../../lib/panelLayout'
import { resetStores } from '../../../stores/defineStore'
import { useApplets } from '../../../stores/useApplets'
import AppletTabsLayer from '../AppletTabsLayer.vue'

const service = vi.hoisted(() => ({
  BridgeToken: vi.fn(() => Promise.resolve('secret')),
  Context: vi.fn(),
  Exec: vi.fn(),
  KillSpawns: vi.fn(),
  ReadFile: vi.fn(),
  SaveStorage: vi.fn(),
  Spawn: vi.fn(),
  SpawnKill: vi.fn(),
  SpawnRead: vi.fn(),
  SpawnWrite: vi.fn(),
  WriteFile: vi.fn(),
  List: vi.fn(),
  SetEnabled: vi.fn(),
  Delete: vi.fn(),
}))

vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice',
  () => service,
)

const radio = {
  id: 'radio',
  title: 'Radio',
  entry: 'index.html',
  revision: 1,
  runnable: true,
  enabled: true,
  surfaces: { tab: true, popup: {}, titlebar: null },
}

async function render(layout: PanelLayout, extra: object = {}) {
  const wrapper = mount(AppletTabsLayer, {
    props: {
      layout,
      region: () => ({}),
      sessionSlug: 'review',
      contentKey: 's1',
      active: true,
      keysActive: true,
      ...extra,
    },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

describe('AppletTabsLayer', () => {
  beforeEach(() => {
    resetStores()
    service.List.mockResolvedValue([radio])
  })

  it('runs nothing in a saved tab once the file stops listing a tab', async () => {
    service.List.mockResolvedValue([{ ...radio, surfaces: { tab: false, popup: {}, titlebar: null } }])
    const wrapper = await render(openTab(newPanelLayout(), appletTab('radio')))
    expect(wrapper.find('[data-testid="applet-tab-frame"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="applet-tab-unavailable"]').text()).toContain('no longer opens as a tab')
    wrapper.unmount()
  })

  it('runs a tab as its own surface, with no close button of its own', async () => {
    const wrapper = await render(openTab(newPanelLayout(), appletTab('radio')))
    expect(wrapper.get('[data-testid="applet-tab-frame"]').attributes('src')).toContain('surface=tab')
    expect(wrapper.find('[data-testid="applet-page-close"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps a tab behind another one running', async () => {
    const behind = openTab(openTab(newPanelLayout(), appletTab('radio')), { id: 'docs', kind: 'docs' })
    const wrapper = await render(behind)
    const view = wrapper.get('[data-testid="panel-view-applet"]')
    expect(view.isVisible()).toBe(false)
    expect(wrapper.find('iframe').exists()).toBe(true)
    wrapper.unmount()
  })

  it('takes focus only while its panel holds the keyboard', async () => {
    const layout = openTab(newPanelLayout(), appletTab('radio'))
    const wrapper = await render(layout, { keysActive: false })
    const frame = wrapper.get('iframe').element
    expect(document.activeElement).not.toBe(frame)
    await wrapper.setProps({ keysActive: true })
    await nextTick()
    expect(document.activeElement).toBe(frame)
    wrapper.unmount()
  })

  it('takes the keyboard when the tab comes forward in the focused panel', async () => {
    const front = openTab(openTab(newPanelLayout(), appletTab('radio')), { id: 'docs', kind: 'docs' })
    const wrapper = await render(front)
    const frame = wrapper.get('iframe').element
    expect(document.activeElement).not.toBe(frame)
    await wrapper.setProps({ layout: selectTab(front, 'p0', 'applet:radio') })
    await nextTick()
    expect(document.activeElement).toBe(frame)
    wrapper.unmount()
  })

  it('does not take a command while a pop-up covers it', async () => {
    const applets = useApplets()
    await flushPromises()
    const wrapper = await render(openTab(newPanelLayout(), appletTab('radio')))
    applets.toggle('radio')
    await nextTick()
    expect(applets.deliverCommand('radio', 'next')).toBe(false)
    applets.close()
    await nextTick()
    expect(applets.deliverCommand('radio', 'next')).toBe(true)
    wrapper.unmount()
  })

  it('leaves the keyboard alone while a pop-up is open', async () => {
    const applets = useApplets()
    await flushPromises()
    const layout = openTab(newPanelLayout(), appletTab('radio'))
    const wrapper = await render(layout, { keysActive: false })
    applets.toggle('radio')
    await wrapper.setProps({ keysActive: true })
    await nextTick()
    expect(document.activeElement).not.toBe(wrapper.get('iframe').element)
    wrapper.unmount()
  })

  it('says so when the applet is off, and keeps the tab', async () => {
    service.List.mockResolvedValue([{ ...radio, enabled: false }])
    const wrapper = await render(openTab(newPanelLayout(), appletTab('radio')))
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.get('[data-testid="applet-tab-unavailable"]').text()).toContain('Radio is turned off')
    wrapper.unmount()
  })

  it('says so when the applet is gone', async () => {
    service.List.mockResolvedValue([])
    const wrapper = await render(openTab(newPanelLayout(), appletTab('radio')))
    expect(wrapper.get('[data-testid="applet-tab-unavailable"]').text()).toContain('radio')
    wrapper.unmount()
  })

  it('closes its tab when the page asks', async () => {
    const wrapper = await render(openTab(newPanelLayout(), appletTab('radio')))
    wrapper.getComponent({ name: 'AppletFrame' }).vm.$emit('close')
    expect(wrapper.emitted('closeTab')?.[0]).toEqual(['p0', 'applet:radio'])
    wrapper.unmount()
  })
})
