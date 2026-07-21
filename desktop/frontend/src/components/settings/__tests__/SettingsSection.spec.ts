import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SettingsSection from '../SettingsSection.vue'

describe('SettingsSection', () => {
  it('renders the title and description', () => {
    const wrapper = mount(SettingsSection, {
      props: { title: 'Actions', description: 'Configure actions.' },
    })

    expect(wrapper.get('h2').text()).toBe('Actions')
    expect(wrapper.get('h2').classes()).toEqual(expect.arrayContaining(['text-[15px]', 'font-semibold', 'text-text']))
    expect(wrapper.get('p').text()).toBe('Configure actions.')
    expect(wrapper.get('p').classes()).toEqual(expect.arrayContaining(['mt-1', 'text-xs', 'leading-relaxed', 'text-text-3']))
  })

  it('renders slot content after the header', () => {
    const wrapper = mount(SettingsSection, {
      props: { title: 'Diagnostics' },
      slots: { default: '<div data-testid="section-body">Body</div>' },
    })

    expect(wrapper.get('[data-testid="section-body"]').text()).toBe('Body')
  })

  it('omits the description when not provided', () => {
    const wrapper = mount(SettingsSection, {
      props: { title: 'Storage locations' },
    })

    expect(wrapper.find('p').exists()).toBe(false)
  })
})
