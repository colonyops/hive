import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EmptyState from '../EmptyState.vue'

describe('EmptyState', () => {
  it('renders a message with plain centered styling', () => {
    const wrapper = mount(EmptyState, {
      props: { message: 'No actions configured.' },
    })

    expect(wrapper.text()).toBe('No actions configured.')
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['py-8', 'text-center', 'text-xs', 'text-text-4']))
    expect(wrapper.classes()).not.toContain('rounded-lg')
  })

  it('renders slot content instead of the message', () => {
    const wrapper = mount(EmptyState, {
      props: { message: 'Fallback' },
      slots: { default: 'No shortcuts match "refresh".' },
    })

    expect(wrapper.text()).toBe('No shortcuts match "refresh".')
  })

  it('adds boxed variant classes', () => {
    const wrapper = mount(EmptyState, {
      props: { boxed: true, message: 'No matches.' },
    })

    expect(wrapper.classes()).toEqual(expect.arrayContaining(['rounded-lg', 'border', 'border-border', 'bg-raised', 'px-4', 'text-text-3']))
    expect(wrapper.classes()).not.toContain('text-text-4')
  })
})
