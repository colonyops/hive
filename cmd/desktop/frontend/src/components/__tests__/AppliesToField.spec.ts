import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppliesToField from '../AppliesToField.vue'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'

function mountField(modelValue: string[] | null = [], knownTypes = ['PR', 'Issue']) {
  return mount(AppliesToField, { props: { modelValue, knownTypes }, attachTo: document.body })
}

describe('AppliesToField', () => {
  it('canonicalises a typed value to the known casing and adds it', async () => {
    const wrapper = mountField()
    const input = wrapper.get('[data-testid="action-applies-to"]')
    await input.setValue('pr')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['PR']])
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
})
