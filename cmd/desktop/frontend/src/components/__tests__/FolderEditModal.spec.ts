import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FolderEditModal from '../FolderEditModal.vue'
import type { FeedFolder } from '../../types/feed'

const work: FeedFolder = {
  id: 'work',
  name: 'Work',
  feeds: [{ id: 'backend', name: 'Backend', count: 1, newCount: 0 }],
}

// The modal teleports to the body.
function el<T extends HTMLElement>(testid: string): T {
  return document.querySelector<T>(`[data-testid="${testid}"]`)!
}

function mountModal(folder: FeedFolder = work) {
  return mount(FolderEditModal, { props: { folder }, attachTo: document.body })
}

beforeEach(() => { document.body.innerHTML = '' })

describe('FolderEditModal', () => {
  it('opens with the name focused and selected, and saves the trimmed name', async () => {
    const wrapper = mountModal()
    await wrapper.vm.$nextTick()

    const input = el<HTMLInputElement>('folder-edit-name')
    expect(document.activeElement).toBe(input)
    expect(input.selectionStart).toBe(0)
    expect(input.selectionEnd).toBe('Work'.length)

    input.value = '  Projects  '
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('folder-edit-save').click()

    expect(wrapper.emitted('save')).toEqual([['Projects']])
    wrapper.unmount()
  })

  it('refuses to save a blank name', async () => {
    const wrapper = mountModal()
    const input = el<HTMLInputElement>('folder-edit-name')
    input.value = '   '
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(el<HTMLButtonElement>('folder-edit-save').disabled).toBe(true)
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    expect(wrapper.emitted('save')).toBeUndefined()
    wrapper.unmount()
  })

  it('keeps delete a quiet footer action that only emits after the inline confirm', async () => {
    const wrapper = mountModal()
    // No filled danger button competing with Save before delete is asked for.
    expect(document.querySelector('[data-testid="folder-delete-confirm"]')).toBeNull()

    el<HTMLButtonElement>('folder-edit-delete').click()
    await wrapper.vm.$nextTick()

    // The confirm strip replaces the footer rather than stacking a dialog. The
    // footer element must go with it — an empty one leaves a bordered bar
    // hanging below the strip.
    expect(el('folder-delete-confirm-description').textContent).toContain('moves to the top level')
    expect(document.querySelector('[data-testid="folder-edit-save"]')).toBeNull()
    expect(document.querySelector('[data-testid="folder-edit-delete"]')).toBeNull()
    expect(document.querySelectorAll('[data-testid="folder-edit-modal"] footer')).toHaveLength(0)
    expect(document.querySelectorAll('[role="alertdialog"]')).toHaveLength(1)
    expect(wrapper.emitted('delete')).toBeUndefined()

    el<HTMLButtonElement>('folder-delete-confirm-confirm').click()
    expect(wrapper.emitted('delete')).toHaveLength(1)
    wrapper.unmount()
  })

  it('makes the rename inert while a delete is pending, and restores it on Keep', async () => {
    const wrapper = mountModal()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('folder-edit-delete').click()
    await wrapper.vm.$nextTick()

    expect(el<HTMLInputElement>('folder-edit-name').disabled).toBe(true)
    // Enter must not reach the dialog's own submit while the confirm is up.
    el('folder-edit-name').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    expect(wrapper.emitted('save')).toBeUndefined()
    // Nor may Escape close the whole dialog out from under the confirm.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()

    // Escape answered the strip instead: back to editing, nothing deleted.
    expect(document.querySelector('[data-testid="folder-delete-confirm"]')).toBeNull()
    expect(el<HTMLInputElement>('folder-edit-name').disabled).toBe(false)
    expect(el<HTMLButtonElement>('folder-edit-save')).not.toBeNull()
    expect(wrapper.emitted('delete')).toBeUndefined()
    wrapper.unmount()
  })

  it('counts what is inside, singular, plural and empty', async () => {
    const one = mountModal()
    expect(el('folder-edit-inside').textContent).toBe('1 feed inside')
    one.unmount()

    const two = mountModal({ ...work, feeds: [...work.feeds, { id: 'ui', name: 'UI', count: 0, newCount: 0 }] })
    expect(el('folder-edit-inside').textContent).toBe('2 feeds inside')
    el<HTMLButtonElement>('folder-edit-delete').click()
    await two.vm.$nextTick()
    expect(el('folder-delete-confirm-description').textContent).toContain('Its 2 feeds move to the top level')
    two.unmount()

    const empty = mountModal({ ...work, feeds: [] })
    expect(el('folder-edit-inside').textContent).toBe('No feeds inside')
    el<HTMLButtonElement>('folder-edit-delete').click()
    await empty.vm.$nextTick()
    expect(el('folder-delete-confirm-description').textContent).toContain('folder is empty')
    empty.unmount()
  })
})
