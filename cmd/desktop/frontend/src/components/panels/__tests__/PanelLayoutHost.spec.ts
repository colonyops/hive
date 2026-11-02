import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { Panel, PanelLayout, PanelTab } from '../../../lib/panelLayout'
import PanelLayoutHost from '../PanelLayoutHost.vue'

const rect = (x: number, y: number, width: number, height: number): DOMRect => ({
  x,
  y,
  left: x,
  top: y,
  right: x + width,
  bottom: y + height,
  width,
  height,
  toJSON: () => ({}),
})

const pointer = (type: string, clientX: number, clientY: number): void => {
  window.dispatchEvent(new PointerEvent(type, { clientX, clientY, pointerId: 1 }))
}

const tab = (id: string, kind = 'note'): PanelTab => ({ id, kind })

function panel(id: string, tabs: PanelTab[], picking = false): Panel {
  return { kind: 'panel', id, tabs, activeTab: tabs[0]?.id ?? null, picking }
}

function twoPanels(second: Panel = panel('p1', [tab('c')])): PanelLayout {
  return {
    root: { kind: 'split', id: 's1', axis: 'row', ratio: 0.5, first: panel('p0', [tab('a'), tab('b')]), second },
    focusedPanel: 'p0',
    nextId: 2,
  }
}

function mountHost(
  layout: PanelLayout,
  extra: Record<string, unknown> = {},
  slots: Record<string, (scope: never) => string> = {},
) {
  const apply = vi.fn<(rule: (layout: PanelLayout) => PanelLayout) => void>()
  const wrapper = mount(PanelLayoutHost, {
    props: { layout, apply, moveTab: vi.fn(), tabLabel: (t: PanelTab) => `label ${t.id}`, ...extra },
    slots,
  })
  const applied = (): PanelLayout => {
    const rule = apply.mock.calls.at(-1)?.[0]
    if (!rule) throw new Error('apply was not called')
    return rule(layout)
  }
  return { wrapper, apply, applied }
}

describe('PanelLayoutHost', () => {
  it('draws a section per panel at its share of the layout', () => {
    const { wrapper } = mountHost(twoPanels())
    expect(wrapper.get('[data-testid="panel-p0"]').attributes('style')).toContain('left: 0%')
    expect(wrapper.get('[data-testid="panel-p0"]').attributes('style')).toContain('width: 50%')
    expect(wrapper.get('[data-testid="panel-p1"]').attributes('style')).toContain('left: 50%')
    expect(wrapper.get('[data-testid="panel-p1"]').attributes('aria-label')).toBe('Panel p1')
  })

  it('splits a panel through apply', async () => {
    const { wrapper, applied } = mountHost(twoPanels())
    await wrapper.get('[data-testid="panel-split-column-p0"]').trigger('click')
    const next = applied()
    expect(next.nextId).toBe(3)
    expect(next.focusedPanel).toBe('p2')
  })

  it('offers close only while there is more than one panel', () => {
    const single: PanelLayout = { root: panel('p0', [tab('a')]), focusedPanel: 'p0', nextId: 1 }
    expect(mountHost(single).wrapper.find('[data-testid="panel-close-p0"]').exists()).toBe(false)
    expect(mountHost(twoPanels()).wrapper.find('[data-testid="panel-close-p0"]').exists()).toBe(true)
  })

  it('draws a divider that resets, turns and steps through apply', async () => {
    const { wrapper, applied } = mountHost(twoPanels())
    const divider = wrapper.get('[data-testid="panel-divider-s1"]')
    expect(divider.attributes('aria-valuenow')).toBe('50')

    await divider.trigger('dblclick')
    expect(applied().root).toMatchObject({ ratio: 0.5 })

    await divider.trigger('keydown', { key: 'ArrowRight' })
    expect(applied().root).toMatchObject({ ratio: 0.55 })

    await wrapper.get('[data-testid="panel-turn-s1"]').trigger('click')
    expect(applied().root).toMatchObject({ axis: 'column' })
  })

  it('emits close-tab from a tab close button and leaves the layout alone', async () => {
    const { wrapper, apply } = mountHost(twoPanels())
    await wrapper.get('[data-testid="panel-tab-close-b"]').trigger('click')
    expect(wrapper.emitted('close-tab')).toEqual([['p0', 'b']])
    expect(apply).not.toHaveBeenCalled()
  })

  it('selects a tab through apply', async () => {
    const { wrapper, applied } = mountHost(twoPanels())
    await wrapper.get('[data-testid="panel-tab-b"] [role="tab"]').trigger('click')
    expect(applied().root).toMatchObject({ first: { activeTab: 'b' } })
  })

  it('focuses a panel on pointerdown', async () => {
    const { wrapper, applied } = mountHost(twoPanels())
    await wrapper.get('[data-testid="panel-p1"]').trigger('pointerdown')
    expect(applied().focusedPanel).toBe('p1')
  })

  it('hands the floating slot the one geometry for a panel content box', () => {
    let region: (panelId: string) => Record<string, string> = () => ({})
    mountHost(
      twoPanels(),
      {},
      {
        floating: (scope: { region: typeof region }) => {
          region = scope.region
          return ''
        },
      },
    )
    expect(region('p0')).toEqual({
      left: 'calc(0% + 0px)',
      top: 'calc(0% + 30px)',
      width: 'calc(50% - 0px)',
      height: 'calc(100% - 30px)',
    })
    expect(region('p1').left).toBe('calc(50% + 1px)')
    expect(region('p1').width).toBe('calc(50% - 1px)')
    expect(region('missing')).toEqual({ inset: '30px 0 0 0' })
  })

  it('wraps only tabs that have content', () => {
    const { wrapper } = mountHost(
      twoPanels(),
      { hasContent: (t: PanelTab) => t.id !== 'a', contentKey: 'k' },
      { tab: (scope: { tab: PanelTab }) => `body ${scope.tab.id}` },
    )
    expect(wrapper.find('[data-testid="panel-view-note"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="panel-view-note"]')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('body a')
    expect(wrapper.text()).toContain('body b')
  })

  it('hides the root without unmounting it', () => {
    expect(mountHost(twoPanels(), { hidden: true }).wrapper.get('[data-testid="panel-layout"]').classes()).toContain(
      'hidden',
    )
    expect(mountHost(twoPanels()).wrapper.get('[data-testid="panel-layout"]').classes()).toContain('block')
  })

  it('renders the picker slot only for a picking panel', () => {
    const picker = { picker: (scope: { panel: Panel }) => `pick ${scope.panel.id}` }
    expect(mountHost(twoPanels(), {}, picker).wrapper.text()).not.toContain('pick')
    const { wrapper } = mountHost(twoPanels(panel('p1', [], true)), {}, picker)
    expect(wrapper.text()).toContain('pick p1')
    expect(wrapper.text()).not.toContain('pick p0')
  })

  it('marks the lead panel for the actions slot', () => {
    const { wrapper } = mountHost(
      twoPanels(),
      {},
      { actions: (scope: { panel: Panel; lead: boolean }) => (scope.lead ? `lead ${scope.panel.id}` : '') },
    )
    expect(wrapper.text()).toContain('lead p1')
    expect(wrapper.text()).not.toContain('lead p0')
  })

  it('shows the dragged tab, washes the panel it would join, and drops it there', async () => {
    const moveTab = vi.fn()
    const wrapper = mount(PanelLayoutHost, {
      attachTo: document.body,
      props: { layout: twoPanels(), apply: vi.fn(), moveTab, tabLabel: (t: PanelTab) => `label ${t.id}` },
      slots: { icon: '<i data-testid="icon" />' },
    })
    // jsdom lays nothing out: two panels side by side, each 400 wide, rows 30 deep.
    const rects: Record<string, DOMRect> = {
      'panel-tab-row-p0': rect(0, 0, 400, 30),
      'panel-tab-row-p1': rect(400, 0, 400, 30),
      'panel-tab-a': rect(0, 0, 100, 30),
      'panel-tab-b': rect(100, 0, 100, 30),
      'panel-tab-c': rect(400, 0, 100, 30),
    }
    const measure = vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
      return rects[this.getAttribute('data-testid') ?? ''] ?? rect(0, 0, 0, 0)
    })
    // Under a row the app's floating layer answers the hit test; the section stands in for it.
    const p1 = document.querySelector('[data-testid="panel-p1"]')
    Object.defineProperty(document, 'elementFromPoint', {
      configurable: true,
      value: (x: number, y: number) => (x >= 400 && y > 30 ? p1 : null),
    })
    const ghost = () => document.body.querySelector('[data-testid="panel-drag-ghost"]')
    try {
      await wrapper
        .get('[data-testid="panel-tab-a"]')
        .trigger('pointerdown', { clientX: 50, clientY: 15, pointerId: 1 })
      pointer('pointermove', 52, 15)
      await nextTick()
      expect(ghost()).toBeNull()

      pointer('pointermove', 600, 200)
      await nextTick()
      expect(wrapper.get('[data-testid="panel-tab-a"]').classes()).toContain('opacity-40')
      expect(ghost()?.textContent).toContain('label a')
      expect(ghost()?.querySelector('[data-testid="icon"]')).not.toBeNull()
      expect(wrapper.get('[data-testid="panel-drop-target"]').attributes('style')).toContain('left: 50%')
      expect(
        wrapper.get('[data-testid="panel-tab-row-p1"]').find('[data-testid="panel-tab-drop-marker"]').exists(),
      ).toBe(true)

      pointer('pointermove', 150, 15)
      await nextTick()
      expect(wrapper.find('[data-testid="panel-drop-target"]').exists()).toBe(false)
      expect(
        wrapper.get('[data-testid="panel-tab-row-p0"]').find('[data-testid="panel-tab-drop-marker"]').exists(),
      ).toBe(true)

      pointer('pointermove', 600, 200)
      pointer('pointerup', 600, 200)
      await nextTick()
      expect(moveTab).toHaveBeenCalledWith('a', 'p1', 1)
      expect(ghost()).toBeNull()
      expect(wrapper.find('[data-testid="panel-drop-target"]').exists()).toBe(false)
      expect(wrapper.get('[data-testid="panel-tab-a"]').classes()).not.toContain('opacity-40')
    } finally {
      measure.mockRestore()
      delete (document as { elementFromPoint?: unknown }).elementFromPoint
      wrapper.unmount()
    }
  })
})
