import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import BaseIconBadge from '../BaseIconBadge.vue'

describe('BaseIconBadge', () => {
  it('sizes its square container and renders the default slot', () => {
    const wrapper = mount(BaseIconBadge, {
      props: { size: 26 },
      attrs: { class: 'bg-chip', 'data-testid': 'icon-badge' },
      slots: { default: () => h('span', { 'data-testid': 'badge-icon' }, 'icon') },
    })

    expect(wrapper.attributes('data-testid')).toBe('icon-badge')
    expect(wrapper.attributes('style')).toContain('width: 26px')
    expect(wrapper.attributes('style')).toContain('height: 26px')
    expect(wrapper.classes()).toContain('bg-chip')
    expect(wrapper.find('[data-testid="badge-icon"]').text()).toBe('icon')
  })
})
