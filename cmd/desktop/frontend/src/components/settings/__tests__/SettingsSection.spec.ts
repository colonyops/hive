import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SettingsSection from '../SettingsSection.vue'

describe('SettingsSection', () => {
  it('renders the title and description', () => {
    const wrapper = mount(SettingsSection, {
      props: { title: 'Actions', description: 'Configure actions.' },
    })

    expect(wrapper.get('h2').text()).toBe('Actions')
    expect(wrapper.get('h2').classes()).toEqual(expect.arrayContaining(['text-xs', 'font-semibold', 'uppercase', 'text-text-2']))
    expect(wrapper.get('p').text()).toBe('Configure actions.')
    expect(wrapper.get('p').classes()).toEqual(expect.arrayContaining(['text-xs', 'leading-relaxed', 'text-text-3']))
  })

  it('boxes the slot into one card only when asked', () => {
    const plain = mount(SettingsSection, {
      props: { title: 'Diagnostics' },
      slots: { default: '<div data-testid="row" />' },
    })
    expect(plain.find('.divide-y').exists()).toBe(false)

    const boxed = mount(SettingsSection, {
      props: { title: 'Diagnostics', boxed: true },
      slots: { default: '<div data-testid="row" />' },
    })
    expect(boxed.get('.divide-y').get('[data-testid="row"]')).toBeTruthy()
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
