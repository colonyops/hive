import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfirmationDialog from '../ConfirmationDialog.vue'

describe('ConfirmationDialog', () => {
  it('emits confirm and cancel, including Escape', async () => {
    const wrapper = mount(ConfirmationDialog, { props: { title: 'Delete', description: 'Delete it?' } })
    document.querySelector<HTMLButtonElement>('[data-testid="confirmation-dialog-confirm"]')!.click()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    wrapper.unmount()
  })

  it('shows backend errors and remains available for retry or cancel', () => {
    const wrapper = mount(ConfirmationDialog, { props: { title: 'Delete', description: 'Delete it?', error: 'action is used by flow', busy: false } })
    expect(document.querySelector('[data-testid="confirmation-dialog-error"]')?.textContent).toContain('used by flow')
    expect(document.querySelector('[data-testid="confirmation-dialog"]')).not.toBeNull()
    wrapper.unmount()
  })
})
