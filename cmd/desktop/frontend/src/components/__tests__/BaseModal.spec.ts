import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import IconPlay from '~icons/lucide/play'
import BaseModal from '../BaseModal.vue'

function mountModal(props: Record<string, unknown> = {}) {
  return mount(BaseModal, {
    props: { title: 'Demo modal', icon: IconPlay, testid: 'demo-modal', ...props },
    slots: {
      default: '<p data-testid="modal-body">Body</p>',
      footer: '<button data-testid="modal-footer">Save</button>',
    },
  })
}

function el<T extends HTMLElement>(testid: string): T {
  const element = document.querySelector<T>(`[data-testid="${testid}"]`)
  if (!element) throw new Error(`Missing ${testid}`)
  return element
}

describe('BaseModal', () => {
  it('renders its title, testids, tone, and footer slot', () => {
    const wrapper = mountModal({ tone: 'danger' })

    expect(el('demo-modal').textContent).toContain('Demo modal')
    expect(el('demo-modal-backdrop')).toBeTruthy()
    expect(el('modal-footer').textContent).toBe('Save')
    const badge = el('demo-modal').querySelector('header span')
    expect(badge?.className).toContain('bg-severity-error-tint')
    expect(badge?.className).toContain('text-severity-error')

    wrapper.unmount()
  })

  it('emits close when its backdrop is clicked', () => {
    const wrapper = mountModal()

    el('demo-modal-backdrop').click()
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })

  it.each([
    { busy: true },
    { closeOnBackdrop: false },
  ])('does not close from the backdrop when %o', (props) => {
    const wrapper = mountModal(props)

    el('demo-modal-backdrop').click()
    expect(wrapper.emitted('close')).toBeUndefined()

    wrapper.unmount()
  })

  it('wraps Tab and Shift+Tab inside the dialog', () => {
    const wrapper = mountModal()
    const first = el<HTMLButtonElement>('demo-modal-close')
    const last = el<HTMLButtonElement>('modal-footer')

    last.focus()
    last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(first)

    first.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(last)

    wrapper.unmount()
  })

  it('pulls focus back in when Tab arrives from outside the dialog', () => {
    const wrapper = mountModal()
    const dialog = el('demo-modal')

    ;(document.activeElement as HTMLElement | null)?.blur()
    dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(el('demo-modal-close'))

    wrapper.unmount()
  })

  it('returns focus to whatever opened it', async () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()

    const wrapper = mountModal()
    el<HTMLButtonElement>('demo-modal-close').focus()
    wrapper.unmount()
    await nextTick()

    expect(document.activeElement).toBe(opener)
    opener.remove()
  })

  it('emits close on Escape unless busy', () => {
    const closable = mountModal()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(closable.emitted('close')).toHaveLength(1)
    closable.unmount()

    const busy = mountModal({ busy: true })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(busy.emitted('close')).toBeUndefined()
    busy.unmount()
  })
})
