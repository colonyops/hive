import { mount, type VueWrapper } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Terminal } from '@xterm/xterm'
import TerminalTab from '../TerminalTab.vue'
import type { TerminalWindowTab, UseTerminalWindows } from '../../composables/useTerminalWindows'
import type { PaneLayout } from '../../lib/terminalClient'
import { layoutLeaves } from '../../lib/terminalLayout'

const sideBySide: PaneLayout = {
  split: 'leftright', x: 0, y: 0, width: 120, height: 40,
  cells: [
    { paneId: '%1', x: 0, y: 0, width: 60, height: 40 },
    { paneId: '%2', x: 61, y: 0, width: 59, height: 40 },
  ],
}

const stacked: PaneLayout = {
  split: 'topbottom', x: 0, y: 0, width: 120, height: 40,
  cells: [
    { paneId: '%1', x: 0, y: 0, width: 120, height: 20 },
    { paneId: '%2', x: 0, y: 21, width: 120, height: 19 },
  ],
}

const single: PaneLayout = { paneId: '%1', x: 0, y: 0, width: 120, height: 40 }

// Both cells stacked on the left are splits, so tmux cannot be told to move
// the root border through any pane.
const unreachable: PaneLayout = {
  split: 'leftright', x: 0, y: 0, width: 120, height: 40,
  cells: [
    {
      split: 'topbottom', x: 0, y: 0, width: 60, height: 40,
      cells: [
        {
          split: 'leftright', x: 0, y: 0, width: 60, height: 20,
          cells: [
            { paneId: '%1', x: 0, y: 0, width: 30, height: 20 },
            { paneId: '%3', x: 31, y: 0, width: 29, height: 20 },
          ],
        },
        {
          split: 'leftright', x: 0, y: 21, width: 60, height: 19,
          cells: [
            { paneId: '%2', x: 0, y: 21, width: 30, height: 19 },
            { paneId: '%4', x: 31, y: 21, width: 29, height: 19 },
          ],
        },
      ],
    },
    { paneId: '%5', x: 61, y: 0, width: 59, height: 40 },
  ],
}

const CELL = { width: 10, height: 25 }
const BOX = { left: 100, top: 50 }

function fakeSession(cell: { width: number; height: number } | null = CELL) {
  return {
    cell: ref(cell),
    attachTab: vi.fn(),
    attachPane: vi.fn(),
    selectPane: vi.fn().mockResolvedValue(undefined),
    resizePane: vi.fn().mockResolvedValue(undefined),
  }
}

function tabWith(layout: PaneLayout, overrides: Partial<TerminalWindowTab> = {}): TerminalWindowTab {
  const panes = layoutLeaves(layout).map((leaf, index) => ({
    uid: index + 1, paneId: leaf.paneId as string, term: {} as Terminal, scrolledUp: false,
  }))
  return {
    uid: 1, windowId: '@1', name: 'shell', active: true, activePane: '%1',
    width: 120, height: 40, zoomed: false, layout, panes, ...overrides,
  }
}

function mountTab(tab: TerminalWindowTab, session = fakeSession()) {
  const wrapper = mount(TerminalTab, {
    props: { tab, session: session as unknown as UseTerminalWindows, active: true },
  })
  const host = wrapper.get('[data-testid="terminal-pane"] > div').element as HTMLElement
  host.getBoundingClientRect = () => ({ ...BOX, x: BOX.left, y: BOX.top, width: 1200, height: 1000, right: 1300, bottom: 1050, toJSON: () => ({}) })
  return { wrapper, session }
}

function pointer(type: string, at: { x?: number; y?: number }): PointerEvent {
  return new PointerEvent(type, { pointerId: 7, clientX: at.x ?? 0, clientY: at.y ?? 0, bubbles: true, cancelable: true })
}

function divider(wrapper: VueWrapper, before: string) {
  return wrapper.get(`[data-testid="terminal-pane-divider"][data-before="${before}"]`)
}

function paneHost(wrapper: VueWrapper, paneId: string): HTMLElement {
  return wrapper.get(`[data-testid="terminal-pane-host"][data-pane-id="${paneId}"]`).element as HTMLElement
}

function columnX(col: number): number {
  return BOX.left + col * CELL.width + 5
}

function rowY(row: number): number {
  return BOX.top + row * CELL.height + 3
}

describe('TerminalTab', () => {
  it('hands the window box and every pane host to the session', () => {
    const { wrapper, session } = mountTab(tabWith(sideBySide))

    expect(session.attachTab).toHaveBeenCalledWith('@1', wrapper.get('[data-testid="terminal-pane"] > div').element)
    expect(session.attachPane.mock.calls.map(([paneId]) => paneId)).toEqual(['%1', '%2'])
  })

  it('places each pane at its cell, in pixels of the measured cell', () => {
    const { wrapper } = mountTab(tabWith(sideBySide))

    const right = paneHost(wrapper, '%2')
    expect(right.style.display).not.toBe('none')
    expect(right.style.left).toBe('610px')
    expect(right.style.width).toBe('590px')
    expect(right.style.height).toBe('1000px')
  })

  it('drags a vertical divider by resizing the pane before it to the pointer column', () => {
    const { wrapper, session } = mountTab(tabWith(sideBySide))

    divider(wrapper, '%1').element.dispatchEvent(pointer('pointerdown', { x: columnX(60) }))
    window.dispatchEvent(pointer('pointermove', { x: columnX(70) }))

    expect(session.resizePane).toHaveBeenCalledTimes(1)
    expect(session.resizePane).toHaveBeenCalledWith('%1', { width: 70 })
  })

  it('drags a horizontal divider by height', () => {
    const { wrapper, session } = mountTab(tabWith(stacked))

    divider(wrapper, '%1').element.dispatchEvent(pointer('pointerdown', { y: rowY(20) }))
    window.dispatchEvent(pointer('pointermove', { y: rowY(30) }))

    expect(session.resizePane).toHaveBeenCalledWith('%1', { height: 30 })
  })

  it('never asks for a pane narrower than one cell or one that swallows its neighbour', () => {
    const { wrapper, session } = mountTab(tabWith(sideBySide))

    divider(wrapper, '%1').element.dispatchEvent(pointer('pointerdown', { x: columnX(60) }))
    window.dispatchEvent(pointer('pointermove', { x: 0 }))
    window.dispatchEvent(pointer('pointermove', { x: columnX(500) }))

    expect(session.resizePane.mock.calls).toEqual([['%1', { width: 1 }], ['%1', { width: 118 }]])
  })

  it('sends nothing while the pointer stays in the cell the divider is already at', () => {
    const { wrapper, session } = mountTab(tabWith(sideBySide))

    divider(wrapper, '%1').element.dispatchEvent(pointer('pointerdown', { x: columnX(60) }))
    window.dispatchEvent(pointer('pointermove', { x: columnX(60) + 3 }))
    expect(session.resizePane).not.toHaveBeenCalled()

    window.dispatchEvent(pointer('pointermove', { x: columnX(70) }))
    window.dispatchEvent(pointer('pointermove', { x: columnX(70) + 4 }))
    expect(session.resizePane).toHaveBeenCalledTimes(1)
  })

  it('stops tracking the pointer after pointerup', () => {
    const { wrapper, session } = mountTab(tabWith(sideBySide))

    divider(wrapper, '%1').element.dispatchEvent(pointer('pointerdown', { x: columnX(60) }))
    window.dispatchEvent(pointer('pointerup', { x: columnX(60) }))
    window.dispatchEvent(pointer('pointermove', { x: columnX(70) }))

    expect(session.resizePane).not.toHaveBeenCalled()
  })

  it('draws a divider no pane can move without a resize cursor and ignores a drag on it', () => {
    const { wrapper, session } = mountTab(tabWith(unreachable))

    const root = divider(wrapper, '')
    expect(root.attributes('data-axis')).toBe('x')
    expect(root.classes()).not.toContain('cursor-col-resize')
    expect(root.classes()).not.toContain('terminal-divider-draggable')
    expect(divider(wrapper, '%2').classes()).toContain('cursor-col-resize')

    root.element.dispatchEvent(pointer('pointerdown', { x: columnX(60) }))
    window.dispatchEvent(pointer('pointermove', { x: columnX(80) }))

    expect(session.resizePane).not.toHaveBeenCalled()
  })

  it('highlights only the dividers beside the active pane', () => {
    const { wrapper } = mountTab(tabWith(unreachable, { activePane: '%4' }))

    const active = wrapper.findAll('[data-testid="terminal-pane-divider"].terminal-divider-active')
      .map((el) => `${el.attributes('data-axis')}:${el.attributes('data-before')}`)
    expect(active).toEqual(['x:', 'y:%1', 'x:%2'])
  })

  it('badges a zoomed window only when the zoom is hiding another pane', () => {
    expect(mountTab(tabWith(sideBySide, { zoomed: true })).wrapper.find('[data-testid="terminal-pane-zoomed"]').exists()).toBe(true)
    expect(mountTab(tabWith(sideBySide)).wrapper.find('[data-testid="terminal-pane-zoomed"]').exists()).toBe(false)
    expect(mountTab(tabWith(single, { zoomed: true })).wrapper.find('[data-testid="terminal-pane-zoomed"]').exists()).toBe(false)
  })

  it('gives the active pane the whole box and hides the rest until a cell is measured', () => {
    const { wrapper } = mountTab(tabWith(sideBySide, { activePane: '%2' }), fakeSession(null))

    const active = paneHost(wrapper, '%2')
    expect(active.style.display).not.toBe('none')
    // Read off the style object: happy-dom accepts the inset shorthand there
    // but never serialises it into the attribute.
    expect(active.style.inset).toBe('0')
    expect(paneHost(wrapper, '%1').style.display).toBe('none')
    expect(wrapper.findAll('[data-testid="terminal-pane-divider"]').every((el) => !el.attributes('style'))).toBe(true)
  })
})
