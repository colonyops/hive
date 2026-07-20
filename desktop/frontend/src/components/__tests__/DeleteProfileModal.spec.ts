import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DeleteProfileModal from '../DeleteProfileModal.vue'

function mountModal(props: { profileName?: string; busy?: boolean } = {}) {
  return mount(DeleteProfileModal, {
    props: { profileName: props.profileName ?? 'Backend Triage', busy: props.busy ?? false },
  })
}

describe('DeleteProfileModal', () => {
  it('names the profile in the body copy', () => {
    const wrapper = mountModal({ profileName: 'Backend Triage' })

    expect(document.body.textContent).toContain('Backend Triage')
    expect(document.body.textContent).toContain('flow file')

    wrapper.unmount()
  })

  it('emits confirm and close', () => {
    const wrapper = mountModal()

    document.querySelector<HTMLButtonElement>('[data-testid="delete-profile-confirm"]')?.click()
    expect(wrapper.emitted('confirm')).toHaveLength(1)

    document.querySelector<HTMLButtonElement>('[data-testid="delete-profile-cancel"]')?.click()
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })

  it('disables the confirm button while busy', () => {
    const wrapper = mountModal({ busy: true })

    expect(document.querySelector<HTMLButtonElement>('[data-testid="delete-profile-confirm"]')?.disabled).toBe(true)

    wrapper.unmount()
  })

  it('closes on Escape', () => {
    const wrapper = mountModal()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })
})
