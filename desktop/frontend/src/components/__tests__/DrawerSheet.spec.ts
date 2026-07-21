import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DrawerSheet from '../DrawerSheet.vue'

function el<T extends HTMLElement>(testid: string): T {
  const element = document.querySelector<T>(`[data-testid="${testid}"]`)
  if (!element) throw new Error(`Missing ${testid}`)
  return element
}

function mountSheet(props: Record<string, unknown> = {}) {
  return mount(DrawerSheet, {
    props: { ariaLabel: 'Demo drawer', testid: 'demo-drawer', ...props },
    slots: {
      header: '<span data-testid="drawer-header">Header</span>',
      default: '<span data-testid="drawer-body">Body</span>',
      footer: '<span data-testid="drawer-footer">Footer</span>',
    },
  })
}

describe('DrawerSheet', () => {
  it('teleports its standard bands and wires the supplied testid to the sheet and backdrop', () => {
    const wrapper = mountSheet()

    expect(el('demo-drawer').getAttribute('aria-label')).toBe('Demo drawer')
    expect(el('demo-drawer-backdrop')).toBeTruthy()
    expect(el('drawer-header').textContent).toBe('Header')
    expect(el('drawer-body').textContent).toBe('Body')
    expect(el('drawer-footer').textContent).toBe('Footer')

    wrapper.unmount()
  })

  it('emits close when its backdrop is clicked', async () => {
    const wrapper = mountSheet()

    el('demo-drawer-backdrop').click()
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })

  it('emits close on Escape unless Escape closing is disabled', () => {
    const closable = mountSheet()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(closable.emitted('close')).toHaveLength(1)
    closable.unmount()

    const nonClosable = mountSheet({ closeOnEscape: false })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(nonClosable.emitted('close')).toBeUndefined()
    nonClosable.unmount()
  })

  it('uses fixed width or a persisted resizable width as configured', () => {
    const fixed = mountSheet({ width: 380 })
    expect(el('demo-drawer').style.width).toBe('380px')
    expect(document.querySelector('[data-testid="resize-handle-demo-drawer"]')).toBeNull()
    fixed.unmount()

    const resizable = mountSheet({ resize: { storageKey: 'hive.panel.drawer-sheet-test', defaultSize: 440, min: 360, max: 760 } })
    expect(el('demo-drawer').style.width).toBe('440px')
    expect(document.querySelector('[data-testid="resize-handle-demo-drawer"]')).not.toBeNull()
    resizable.unmount()
  })

  it('traps Tab navigation by default', () => {
    const wrapper = mount(DrawerSheet, {
      props: { ariaLabel: 'Focus drawer', testid: 'focus-drawer' },
      slots: { default: '<button data-testid="first-focus">First</button><button data-testid="last-focus">Last</button>' },
    })
    const first = el<HTMLButtonElement>('first-focus')
    const last = el<HTMLButtonElement>('last-focus')

    last.focus()
    last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(first)

    first.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(last)
    wrapper.unmount()
  })
})
