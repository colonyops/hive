import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppMenu from '../AppMenu.vue'
import IconMail from '~icons/lucide/mail'
import type { MenuEntry } from '../../types/menu'

const entries: MenuEntry[] = [
  { kind: 'action', id: 'read', label: 'Mark as read', icon: IconMail, kbd: '⇧U', testid: 'entry-read' },
  { kind: 'separator' },
  { kind: 'label', text: 'Actions' },
  { kind: 'action', id: 'run', label: 'Summarize', iconName: 'play', iconColor: '#34d399', testid: 'entry-run' },
]

describe('AppMenu', () => {
  it('renders actions, separators, group labels, and shortcut hints from the entry model', () => {
    const wrapper = mount(AppMenu, { props: { entries } })
    expect(wrapper.findAll('[role="menuitem"]')).toHaveLength(2)
    expect(wrapper.find('.app-menu-sep').exists()).toBe(true)
    expect(wrapper.get('.app-menu-label').text()).toBe('Actions')
    expect(wrapper.get('[data-testid="entry-read"] .app-menu-kbd').text()).toBe('⇧U')
    expect(wrapper.get('[data-testid="entry-run"] svg').attributes('style')).toContain('color:')
  })

  it('emits select with the entry id and close on Escape', async () => {
    const wrapper = mount(AppMenu, { props: { entries }, attachTo: document.body })
    await wrapper.get('[data-testid="entry-run"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([['run']])
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  // An unchecked toggle still has to hold the check's place, or its label sits
  // further left than every icon-bearing entry around it.
  it('marks a toggling entry as a checkbox, and reserves the glyph while it is off', () => {
    const wrapper = mount(AppMenu, {
      props: {
        entries: [
          { kind: 'action', id: 'on', label: 'On', checked: true, testid: 'entry-on' },
          { kind: 'action', id: 'off', label: 'Off', checked: false, testid: 'entry-off' },
        ] satisfies MenuEntry[],
      },
    })
    expect(wrapper.findAll('[role="menuitemcheckbox"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="entry-on"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.find('[data-testid="entry-on"] svg').exists()).toBe(true)
    expect(wrapper.find('[data-testid="entry-off"] svg').exists()).toBe(false)
    expect(wrapper.get('[data-testid="entry-off"] span').classes()).toContain('size-3.5')
  })

  it('flips upward when asked to open above its anchor', () => {
    const wrapper = mount(AppMenu, { props: { entries, flip: true } })
    expect(wrapper.get('.app-menu').classes()).toContain('flip')
  })
})
