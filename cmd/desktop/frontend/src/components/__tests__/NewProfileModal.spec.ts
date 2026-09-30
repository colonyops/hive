import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NewProfileModal from '../NewProfileModal.vue'

function mountModal(props: { busy?: boolean; error?: string | null } = {}) {
  return mount(NewProfileModal, { props: { busy: props.busy ?? false, error: props.error ?? null } })
}

function input(): HTMLInputElement {
  const el = document.querySelector<HTMLInputElement>('[data-testid="new-profile-input"]')
  if (!el) throw new Error('input not rendered')
  return el
}

function submitButton(): HTMLButtonElement {
  const el = document.querySelector<HTMLButtonElement>('[data-testid="new-profile-submit"]')
  if (!el) throw new Error('submit not rendered')
  return el
}

describe('NewProfileModal', () => {
  it('emits create with the trimmed name', async () => {
    const wrapper = mountModal()

    input().value = '  Frontend Triage  '
    input().dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()
    submitButton().click()

    expect(wrapper.emitted('create')).toEqual([['Frontend Triage']])

    wrapper.unmount()
  })

  it('disables submit while empty or busy', async () => {
    const wrapper = mountModal()
    expect(submitButton().disabled).toBe(true)

    input().value = 'Work'
    input().dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()
    expect(submitButton().disabled).toBe(false)

    await wrapper.setProps({ busy: true })
    expect(submitButton().disabled).toBe(true)
    submitButton().click()
    expect(wrapper.emitted('create')).toBeUndefined()

    wrapper.unmount()
  })

  it('shows the error and closes on Escape', () => {
    const wrapper = mountModal({ error: 'boom' })

    expect(document.querySelector('[data-testid="new-profile-error"]')?.textContent).toBe('boom')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })
})
