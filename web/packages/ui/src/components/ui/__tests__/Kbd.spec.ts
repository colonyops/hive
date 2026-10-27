import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Kbd from '../Kbd.vue'

describe('Kbd', () => {
  it('renders its slot as a kbd element', () => {
    const wrapper = mount(Kbd, { slots: { default: '⌘K' } })

    expect(wrapper.element.tagName).toBe('KBD')
    expect(wrapper.text()).toBe('⌘K')
  })

  it('passes attributes through to the kbd', () => {
    const wrapper = mount(Kbd, {
      props: { variant: 'plain' },
      attrs: { 'data-testid': 'hint' },
      slots: { default: '↵' },
    })

    expect(wrapper.get('[data-testid="hint"]').text()).toBe('↵')
  })
})
