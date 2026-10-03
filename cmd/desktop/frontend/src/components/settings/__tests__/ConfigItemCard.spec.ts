import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import IconTerminal from '~icons/lucide/terminal'
import ConfigItemCard from '../ConfigItemCard.vue'

describe('ConfigItemCard', () => {
  it('renders the title, the leading and badge slots, and passes attributes to the card', () => {
    const wrapper = mount(ConfigItemCard, {
      props: { title: 'Lazygit', icon: IconTerminal },
      attrs: { 'data-testid': 'launcher-row-lazygit', draggable: 'true' },
      slots: {
        leading: '<span data-testid="grip" />',
        badges: '<span data-testid="badge">lazygit</span>',
      },
    })

    const card = wrapper.get('[data-testid="launcher-row-lazygit"]')
    expect(card.text()).toContain('Lazygit')
    expect(card.attributes('draggable')).toBe('true')
    expect(card.find('[data-testid="grip"]').exists()).toBe(true)
    expect(card.get('[data-testid="badge"]').text()).toBe('lazygit')
  })

  it('emits edit with the click and delete from its buttons', async () => {
    const wrapper = mount(ConfigItemCard, { props: { title: 'Lazygit', icon: IconTerminal } })

    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('edit')?.[0]?.[0]).toBeInstanceOf(MouseEvent)

    await wrapper.get('button[aria-label="Delete"]').trigger('click')
    expect(wrapper.emitted('delete')).toHaveLength(1)
  })
})
