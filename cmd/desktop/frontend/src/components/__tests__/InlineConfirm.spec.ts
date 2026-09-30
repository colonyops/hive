import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import InlineConfirm from '../InlineConfirm.vue'

function mountConfirm(props: Record<string, unknown> = {}) {
  return mount(InlineConfirm, {
    props: { title: 'Delete this folder?', description: 'Its 3 feeds move to the top level.', ...props },
    attachTo: document.body,
  })
}

beforeEach(() => { document.body.innerHTML = '' })

describe('InlineConfirm', () => {
  it('states the consequence and answers confirm and cancel', async () => {
    const wrapper = mountConfirm()
    expect(wrapper.get('[data-testid="inline-confirm-description"]').text()).toBe('Its 3 feeds move to the top level.')
    expect(wrapper.get('[data-testid="inline-confirm"]').attributes('role')).toBe('alertdialog')

    await wrapper.get('[data-testid="inline-confirm-confirm"]').trigger('click')
    expect(wrapper.emitted('confirm')).toHaveLength(1)
    await wrapper.get('[data-testid="inline-confirm-cancel"]').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    wrapper.unmount()
  })

  it('focuses the safe answer, not the destructive one, and cancels on Escape', async () => {
    const wrapper = mountConfirm()
    await wrapper.vm.$nextTick()

    expect(document.activeElement).toBe(wrapper.get('[data-testid="inline-confirm-cancel"]').element)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('confirm')).toBeUndefined()
    wrapper.unmount()
  })

  it('locks both answers while busy and surfaces an error without closing', async () => {
    const wrapper = mountConfirm({ busy: true })
    const confirm = wrapper.get('[data-testid="inline-confirm-confirm"]').element as HTMLButtonElement
    const cancel = wrapper.get('[data-testid="inline-confirm-cancel"]').element as HTMLButtonElement
    expect(confirm.disabled).toBe(true)
    expect(cancel.disabled).toBe(true)
    expect(confirm.textContent).toContain('Working…')

    // A busy strip must not answer Escape either — the action is already in flight.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('cancel')).toBeUndefined()

    await wrapper.setProps({ busy: false, error: 'flow-a blocks deletion' })
    expect(wrapper.get('[data-testid="inline-confirm-error"]').text()).toContain('flow-a blocks deletion')
    expect(wrapper.find('[data-testid="inline-confirm-confirm"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('takes its labels and testid from the host so other surfaces can reuse it', () => {
    const wrapper = mountConfirm({ confirmLabel: 'Revoke token', cancelLabel: 'Back', testid: 'revoke-confirm' })
    expect(wrapper.get('[data-testid="revoke-confirm-confirm"]').text()).toBe('Revoke token')
    expect(wrapper.get('[data-testid="revoke-confirm-cancel"]').text()).toBe('Back')
    wrapper.unmount()
  })
})
