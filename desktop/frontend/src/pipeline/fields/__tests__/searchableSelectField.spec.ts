import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { markRaw } from 'vue'
import SearchableSelectField from '../SearchableSelectField.vue'

const Dot = markRaw({ template: '<i class="dot" />' })
const options = [
  { value: 'git-branch', label: 'Branch', icon: Dot },
  { value: 'sparkles', label: 'AI / generated', icon: Dot },
  { value: 'bug', label: 'Bugs', icon: Dot },
]

function open(wrapper: ReturnType<typeof mount>) {
  return wrapper.get('[data-testid="icon"]').trigger('click')
}

describe('SearchableSelectField', () => {
  it('shows the selected option label and opens/closes the picker', async () => {
    const wrapper = mount(SearchableSelectField, { props: { modelValue: 'sparkles', options, testid: 'icon' } })
    expect(wrapper.get('[data-testid="icon"]').text()).toContain('AI / generated')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    await open(wrapper)
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)
  })

  it('filters options by the search query', async () => {
    const wrapper = mount(SearchableSelectField, { props: { modelValue: 'git-branch', options, testid: 'icon' } })
    await open(wrapper)
    await wrapper.get('[data-testid="icon-search"]').setValue('bug')
    expect(wrapper.find('[data-testid="icon-option-bug"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="icon-option-sparkles"]').exists()).toBe(false)
  })

  it('shows an empty state when nothing matches', async () => {
    const wrapper = mount(SearchableSelectField, { props: { modelValue: 'git-branch', options, testid: 'icon' } })
    await open(wrapper)
    await wrapper.get('[data-testid="icon-search"]').setValue('zzz')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="icon-empty"]').text()).toContain('No matches')
  })

  it('emits the clicked option and closes', async () => {
    const wrapper = mount(SearchableSelectField, { props: { modelValue: 'git-branch', options, testid: 'icon' } })
    await open(wrapper)
    await wrapper.get('[data-testid="icon-option-bug"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['bug'])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('selects the keyboard-active option from the filtered list', async () => {
    const wrapper = mount(SearchableSelectField, { props: { modelValue: 'git-branch', options, testid: 'icon' } })
    await open(wrapper)
    const search = wrapper.get('[data-testid="icon-search"]')
    await search.setValue('b') // Branch, Bugs
    await search.trigger('keydown', { key: 'ArrowDown' }) // active -> Bugs
    await search.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['bug'])
  })

  it('does not re-emit when the current value is chosen', async () => {
    const wrapper = mount(SearchableSelectField, { props: { modelValue: 'bug', options, testid: 'icon' } })
    await open(wrapper)
    await wrapper.get('[data-testid="icon-option-bug"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
