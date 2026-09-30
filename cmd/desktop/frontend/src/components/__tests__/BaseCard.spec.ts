import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import BaseCard from '../BaseCard.vue'

describe('BaseCard', () => {
  it('renders as an article with its icon, body, and actions slots', () => {
    const wrapper = mount(BaseCard, {
      slots: {
        icon: () => h('span', { 'data-testid': 'card-icon' }, 'icon'),
        default: () => h('div', { 'data-testid': 'card-body' }, 'body'),
        actions: () => h('button', { 'data-testid': 'card-actions' }, 'actions'),
      },
    })

    expect(wrapper.element.tagName).toBe('ARTICLE')
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['flex', 'items-center', 'gap-3', 'p-4']))
    expect(wrapper.find('[data-testid="card-icon"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="card-body"]').text()).toBe('body')
    expect(wrapper.find('[data-testid="card-actions"]').text()).toBe('actions')
  })

  it('renders as a button and forwards attributes and clicks to its root', async () => {
    const onClick = vi.fn()
    const wrapper = mount(BaseCard, {
      props: { as: 'button', interactive: true, padded: false },
      attrs: { 'data-testid': 'interactive-card', onClick },
    })

    expect(wrapper.element.tagName).toBe('BUTTON')
    expect(wrapper.attributes('data-testid')).toBe('interactive-card')
    expect(wrapper.classes()).toContain('hover:border-strong')
    expect(wrapper.classes()).not.toContain('p-4')

    await wrapper.trigger('click')
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})
