import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import SegmentedControl from '../SegmentedControl.vue'

const options = [
  { value: 'all', label: 'All' },
  { value: 'unread', label: 'Unread', title: 'Unread items' },
]

describe('SegmentedControl', () => {
  it('marks the selected option pressed and emits the clicked value', async () => {
    const wrapper = mount(SegmentedControl, { props: { modelValue: 'all', options, testid: 'filter' } })

    expect(wrapper.get('[data-testid="filter-all"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="filter-unread"]').attributes('aria-pressed')).toBe('false')

    await wrapper.get('[data-testid="filter-unread"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['unread']])
  })

  it('shows the field label but no root test id in the field variant', () => {
    const wrapper = mount(SegmentedControl, { props: { modelValue: 'all', options, label: 'Show', testid: 'filter' } })

    expect(wrapper.text()).toContain('Show')
    expect(wrapper.find('[data-testid="filter"]').exists()).toBe(false)
  })

  it('names the compact strip by its test id and its options by their title', () => {
    const wrapper = mount(SegmentedControl, {
      props: { modelValue: 'all', options, variant: 'compact', ariaLabel: 'Filter', testid: 'filter' },
    })

    expect(wrapper.get('[data-testid="filter"]').attributes('aria-label')).toBe('Filter')
    expect(wrapper.get('[data-testid="filter-unread"]').attributes('aria-label')).toBe('Unread items')
  })

  it('renders custom option content through the option slot', () => {
    const wrapper = mount(SegmentedControl, {
      props: { modelValue: 'unread', options, variant: 'compact', testid: 'filter' },
      slots: {
        option: ({ option, selected }: { option: { label: string }; selected: boolean }) =>
          h('span', `${option.label}${selected ? '*' : ''}`),
      },
    })

    expect(wrapper.get('[data-testid="filter-unread"]').text()).toBe('Unread*')
    expect(wrapper.get('[data-testid="filter-all"]').text()).toBe('All')
  })
})
