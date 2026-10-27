import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EmptyState from '../EmptyState.vue'

describe('EmptyState', () => {
  it('renders its message', () => {
    const wrapper = mount(EmptyState, { props: { message: 'No actions configured.' } })

    expect(wrapper.text()).toBe('No actions configured.')
  })

  it('renders slot content instead of the message', () => {
    const wrapper = mount(EmptyState, {
      props: { message: 'Fallback', variant: 'boxed' },
      slots: { default: 'No shortcuts match "refresh".' },
    })

    expect(wrapper.text()).toBe('No shortcuts match "refresh".')
  })

  it('passes attributes through to its root', () => {
    const wrapper = mount(EmptyState, {
      props: { message: 'No servers yet.', variant: 'inline' },
      attrs: { 'data-testid': 'servers-empty' },
    })

    expect(wrapper.attributes('data-testid')).toBe('servers-empty')
  })
})
