import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { useOpenModalCount } from '../../composables/useOpenModalCount'
import DrawerSheet from '../DrawerSheet.vue'
import SettingsLayout from '../settings/SettingsLayout.vue'

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

  it('is resizable by default and fixed only when width is given', () => {
    const resizable = mountSheet()
    expect(el('demo-drawer').style.width).toBe('440px')
    expect(document.querySelector('[data-testid="resize-handle-demo-drawer"]')).not.toBeNull()
    resizable.unmount()

    const sized = mountSheet({ defaultSize: 500, storageKey: 'hive.panel.drawer-sheet-test' })
    expect(el('demo-drawer').style.width).toBe('500px')
    sized.unmount()

    const fixed = mountSheet({ width: 380 })
    expect(el('demo-drawer').style.width).toBe('380px')
    expect(document.querySelector('[data-testid="resize-handle-demo-drawer"]')).toBeNull()
    fixed.unmount()
  })

  it('traps Tab navigation by default', () => {
    const wrapper = mount(DrawerSheet, {
      // Fixed width: the resize handle is focusable and would otherwise be
      // the trap's first tab stop, which is beside the point here.
      props: { ariaLabel: 'Focus drawer', testid: 'focus-drawer', width: 400 },
      slots: {
        default: '<button data-testid="first-focus">First</button><button data-testid="last-focus">Last</button>',
      },
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

  it('closes on Escape without also closing the settings page behind it', async () => {
    const drawerOpen = ref(true)
    const closeDrawer = vi.fn(() => (drawerOpen.value = false))
    const Host = defineComponent({
      emits: ['close-settings'],
      setup(_, { emit }) {
        return () =>
          h(SettingsLayout, { onClose: () => emit('close-settings') }, () =>
            drawerOpen.value ? h(DrawerSheet, { ariaLabel: 'Nested drawer', onClose: closeDrawer }) : null,
          )
      },
    })
    const wrapper = mount(Host)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(closeDrawer).toHaveBeenCalledOnce()
    expect(wrapper.emitted('close-settings')).toBeUndefined()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close-settings')).toHaveLength(1)
    wrapper.unmount()
  })

  it('counts as an open modal while mounted', () => {
    const count = useOpenModalCount()
    const wrapper = mountSheet()
    expect(count.value).toBe(1)
    wrapper.unmount()
    expect(count.value).toBe(0)
  })

  it('takes focus when it opens and hands it back when it closes', async () => {
    const trigger = document.createElement('button')
    document.body.append(trigger)
    trigger.focus()

    const wrapper = mountSheet({ width: 400 })
    await flushPromises()
    expect(document.activeElement).toBe(el('demo-drawer'))

    wrapper.unmount()
    await nextTick()
    expect(document.activeElement).toBe(trigger)
    trigger.remove()
  })
})
