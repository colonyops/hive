import { describe, expect, it } from 'vitest'
import { DOMWrapper, mount } from '@vue/test-utils'
import AppliesToField from '../AppliesToField.vue'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'

// The suggestion list is teleported to <body>.
const body = () => new DOMWrapper(document.body)

function mountField(modelValue: string[] | null = [], knownTypes = ['PR', 'Issue']) {
  return mount(AppliesToField, { props: { modelValue, knownTypes }, attachTo: document.body })
}

describe('AppliesToField', () => {
  it('suggests known types on click and filters them by the draft', async () => {
    const wrapper = mountField()
    const input = wrapper.get('[data-testid="action-applies-to"]')
    await input.trigger('click')
    expect(body().get('[data-testid="action-applies-to-suggestions"]').isVisible()).toBe(true)
    expect(body().find('[data-testid="action-applies-to-option-PR"]').exists()).toBe(true)

    await input.setValue('iss')
    expect(body().find('[data-testid="action-applies-to-option-Issue"]').exists()).toBe(true)
    expect(body().find('[data-testid="action-applies-to-option-PR"]').exists()).toBe(false)
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
    await wrapper.get('[data-testid="action-applies-to"]').trigger('click')
    await body().get('[data-testid="action-applies-to-option-Issue"]').trigger('mousedown')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['Issue']])
    wrapper.unmount()
  })

  it('flags configured types that no longer match a known kind', () => {
    const known = mountField(['PR'])
    expect(known.text()).not.toContain('match any known feed item')
    known.unmount()
    const unknown = mountField(['bogus'])
    expect(unknown.getComponent(AppTooltip).props('text')).toContain('Not a known feed-item type')
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
