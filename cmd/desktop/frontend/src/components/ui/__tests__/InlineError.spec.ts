import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import InlineError from '../InlineError.vue'

describe('InlineError', () => {
  it('renders the message as an alert under the given test id', () => {
    const wrapper = mount(InlineError, { props: { message: 'Save failed', testid: 'thing-error' } })

    expect(wrapper.text()).toBe('Save failed')
    expect(wrapper.attributes('role')).toBe('alert')
    expect(wrapper.attributes('data-testid')).toBe('thing-error')
  })

  it('renders slot content instead of the message', () => {
    const wrapper = mount(InlineError, {
      props: { message: 'Fallback' },
      slots: { default: '<p>First</p><p>Second</p>' },
    })

    expect(wrapper.findAll('p').map((p) => p.text())).toEqual(['First', 'Second'])
  })

  it('accepts a null message', () => {
    const wrapper = mount(InlineError, { props: { message: null, variant: 'line' } })

    expect(wrapper.text()).toBe('')
    expect(wrapper.attributes('role')).toBe('alert')
  })

  it('omits the test id when none is given', () => {
    const wrapper = mount(InlineError, { props: { message: 'x' } })

    expect(wrapper.attributes('data-testid')).toBeUndefined()
  })
})
