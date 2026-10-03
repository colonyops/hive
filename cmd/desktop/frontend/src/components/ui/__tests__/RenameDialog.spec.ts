import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import RenameDialog from '../RenameDialog.vue'

// The dialog teleports to the body.
function el<T extends HTMLElement>(testid: string): T | null {
  return document.querySelector<T>(`[data-testid="${testid}"]`)
}

function mountDialog(props: Record<string, unknown> = {}, slots: Record<string, () => unknown> = {}) {
  return mount(RenameDialog, {
    props: { title: 'Rename thing', label: 'Name', name: 'Work', testid: 'thing-rename', ...props },
    slots,
    attachTo: document.body,
  })
}

function type(value: string): void {
  const input = el<HTMLInputElement>('thing-rename-input')!
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('RenameDialog', () => {
  it('opens with the name focused and selected, and saves the trimmed name', async () => {
    const wrapper = mountDialog()
    await wrapper.vm.$nextTick()

    const input = el<HTMLInputElement>('thing-rename-input')!
    expect(document.activeElement).toBe(input)
    expect(input.selectionStart).toBe(0)
    expect(input.selectionEnd).toBe('Work'.length)

    type('  Projects  ')
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('thing-rename-save')!.click()
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))

    expect(wrapper.emitted('save')).toEqual([['Projects'], ['Projects']])
    wrapper.unmount()
  })

  it('refuses a blank name', async () => {
    const wrapper = mountDialog()
    type('   ')
    await wrapper.vm.$nextTick()

    expect(el<HTMLButtonElement>('thing-rename-save')!.disabled).toBe(true)
    el('thing-rename-input')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    expect(wrapper.emitted('save')).toBeUndefined()
    wrapper.unmount()
  })

  it('disables the form while busy and shows the error in place of the hint', () => {
    const wrapper = mountDialog({ busy: true, hint: 'A hint', error: 'Name taken' })

    expect(el<HTMLInputElement>('thing-rename-input')!.disabled).toBe(true)
    expect(el<HTMLButtonElement>('thing-rename-cancel')!.disabled).toBe(true)
    el('thing-rename-input')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    expect(wrapper.emitted('save')).toBeUndefined()
    expect(el('thing-rename-error')!.textContent).toContain('Name taken')
    expect(el('thing-rename-hint')).toBeNull()
    wrapper.unmount()
  })

  it('closes on Cancel and on Escape', () => {
    const wrapper = mountDialog()
    el<HTMLButtonElement>('thing-rename-cancel')!.click()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(2)
    wrapper.unmount()
  })

  it('derives its ids from testid, with overrides for older ids', () => {
    const plain = mountDialog()
    expect(el('thing-rename-dialog')).not.toBeNull()
    plain.unmount()

    const legacy = mountDialog({ testids: { modal: 'thing-modal', input: 'thing-name', save: 'thing-submit' } })
    expect(el('thing-modal')).not.toBeNull()
    expect(el('thing-name')).not.toBeNull()
    expect(el('thing-submit')).not.toBeNull()
    expect(el('thing-rename-cancel')).not.toBeNull()
    legacy.unmount()
  })

  it('renders footer-start beside the buttons', () => {
    const wrapper = mountDialog({}, { 'footer-start': () => h('button', { 'data-testid': 'extra' }, 'Delete') })
    expect(el('extra')).not.toBeNull()
    wrapper.unmount()
  })

  it('locked: the form is inert, the footer is gone, and Escape does not close it', async () => {
    const wrapper = mountDialog({ locked: true }, { default: () => h('p', { 'data-testid': 'pending' }, 'Sure?') })
    await wrapper.vm.$nextTick()

    expect(el('pending')).not.toBeNull()
    expect(el<HTMLInputElement>('thing-rename-input')!.disabled).toBe(true)
    expect(el('thing-rename-save')).toBeNull()
    expect(document.querySelectorAll('[data-testid="thing-rename-dialog"] footer')).toHaveLength(0)
    el('thing-rename-input')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('save')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })
})
