import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TreeRail from '../TreeRail.vue'

describe('TreeRail', () => {
  it('sits at the measured offset and says whether it is shown', () => {
    const wrapper = mount(TreeRail, { props: { rail: { y: 42, height: 30, shown: true } } })
    expect(wrapper.attributes('data-shown')).toBe('true')
    expect(wrapper.attributes('style')).toContain('translateY(42px)')
    expect(wrapper.attributes('style')).toContain('height: 30px')
    expect(wrapper.attributes('aria-hidden')).toBe('true')
  })
})
