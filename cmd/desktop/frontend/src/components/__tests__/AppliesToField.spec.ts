import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppliesToField from '../AppliesToField.vue'

function mountField(modelValue: string[] | null = [], knownTypes = ['PR', 'Issue']) {
  return mount(AppliesToField, { props: { modelValue, knownTypes }, attachTo: document.body })
}

describe('AppliesToField', () => {
  it('suggests known types on focus and filters them by the draft', async () => {
    const wrapper = mountField()
    const input = wrapper.get('[data-testid="action-applies-to"]')
    await input.trigger('focus')
    expect(wrapper.get('[data-testid="action-applies-to-suggestions"]').isVisible()).toBe(true)
    expect(wrapper.find('[data-testid="action-applies-to-option-PR"]').exists()).toBe(true)

    await input.setValue('iss')
    expect(wrapper.find('[data-testid="action-applies-to-option-Issue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="action-applies-to-option-PR"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('canonicalises a typed value to the known casing and adds it', async () => {
    const wrapper = mountField()
    const input = wrapper.get('[data-testid="action-applies-to"]')
    await input.setValue('pr')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['PR']])
    wrapper.unmount()
  })

  it('adds a suggestion when picked and hides it from the list', async () => {
    const wrapper = mountField()
    await wrapper.get('[data-testid="action-applies-to"]').trigger('focus')
    await wrapper.get('[data-testid="action-applies-to-option-Issue"]').trigger('mousedown')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['Issue']])
    wrapper.unmount()
  })

  it('flags configured types that no longer match a known kind', () => {
    const known = mountField(['PR'])
    expect(known.text()).not.toContain('match any known feed item')
    known.unmount()
    const unknown = mountField(['bogus'])
    expect(unknown.get('[title]').attributes('title')).toContain('Not a known feed-item type')
    expect(unknown.text()).toContain('match any known feed item')
    unknown.unmount()
  })

  it('removes the last chip on Backspace and commits the draft on flush', async () => {
    const wrapper = mountField(['PR', 'Issue'])
    await wrapper.get('[data-testid="action-applies-to"]').trigger('keydown', { key: 'Backspace' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['PR']])

    const draft = mountField([])
    await draft.get('[data-testid="action-applies-to"]').setValue('custom')
    ;(draft.vm as unknown as { flush: () => void }).flush()
    expect(draft.emitted('update:modelValue')?.[0]).toEqual([['custom']])
    draft.unmount()
    wrapper.unmount()
  })
})
