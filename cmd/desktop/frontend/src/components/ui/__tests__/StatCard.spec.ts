import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import StatCard from '../StatCard.vue'

describe('StatCard', () => {
  it('renders a value, unit, hint, detail, and chart slot', async () => {
    const wrapper = mount(StatCard, {
      props: { label: 'Memory', value: '4.2', unit: ' GB', hint: 'resident', testid: 'memory' },
      slots: { detail: 'peak 5 GB', default: '<svg data-testid="chart" />' },
    })

    expect(wrapper.get('[data-testid="memory"]').text()).toContain('Memory')
    expect(wrapper.get('[data-testid="memory-value"]').text()).toContain('4.2')
    expect(wrapper.get('[data-testid="memory-value"] span').text()).toBe('GB')
    expect(wrapper.text()).toContain('resident')
    expect(wrapper.text()).toContain('peak 5 GB')
    expect(wrapper.find('[data-testid="chart"]').exists()).toBe(true)

    await wrapper.setProps({ value: '4.3' })
    expect(wrapper.get('[data-testid="memory-value"]').text()).toContain('4.3')
  })

  it('renders a plain count and keeps its existing value test ID', () => {
    const wrapper = mount(StatCard, {
      props: { label: 'CLI commands', value: 0, valueTestid: 'analytics-cli-count' },
    })

    expect(wrapper.get('[data-testid="analytics-cli-count"]').text()).toBe('0')
    expect(wrapper.find('[data-testid="analytics-cli-count"] span').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(false)
  })
})
