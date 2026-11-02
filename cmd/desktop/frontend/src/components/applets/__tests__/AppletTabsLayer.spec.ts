import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { appletTab } from '../../../lib/appletTabs'
import { newPanelLayout, openTab, type PanelLayout } from '../../../lib/panelLayout'
import { resetStores } from '../../../stores/defineStore'
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

const radio = { id: 'radio', title: 'Radio', entry: 'index.html', revision: 1, runnable: true, enabled: true }

async function render(layout: PanelLayout, extra: object = {}) {
  const wrapper = mount(AppletTabsLayer, {
    props: {
      layout,
      region: () => ({}),
      sessionSlug: 'review',
      contentKey: 's1',
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
    expect(document.activeElement).toBe(frame)
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
