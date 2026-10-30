import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppMenu from '../AppMenu.vue'
import IconMail from '~icons/lucide/mail'
import type { MenuEntry } from '../../../types/menu'

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
    expect(wrapper.get('[data-testid="entry-read"] kbd').text()).toBe('⇧U')
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

  it('keeps an icon on a toggling entry and moves the check to the trailing end', () => {
    const wrapper = mount(AppMenu, {
      props: {
        entries: [
          { kind: 'action', id: 'on', label: 'On', icon: IconMail, checked: true, testid: 'entry-on' },
          { kind: 'action', id: 'off', label: 'Off', icon: IconMail, checked: false, testid: 'entry-off' },
        ] satisfies MenuEntry[],
      },
    })
    const on = wrapper.get('[data-testid="entry-on"]')
    expect(on.findAll('svg')).toHaveLength(2)
    expect(on.element.lastElementChild?.hasAttribute('data-check')).toBe(true)
    expect(wrapper.find('[data-testid="entry-off"] [data-check]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="entry-off"]').element.lastElementChild?.classList.contains('size-3.5')).toBe(true)
  })

  // A host at the window's left edge would push an end-aligned panel, and the
  // submenu that opens leftward from it, off-screen.
  it('aligns to the start edge, and opens its submenus toward it', async () => {
    const wrapper = mount(AppMenu, {
      props: {
        align: 'start',
        entries: [
          {
            kind: 'submenu',
            id: 'more',
            label: 'More',
            testid: 'entry-more',
            panelTestid: 'more-panel',
            entries: [{ kind: 'action', id: 'x', label: 'X' }],
          },
        ],
      },
    })
    expect(wrapper.get('[role="menu"]').classes()).toContain('app-menu-start')
    await wrapper.get('[data-testid="entry-more"]').trigger('click')
    expect(wrapper.get('[data-testid="more-panel"]').classes()).toEqual(
      expect.arrayContaining(['app-menu-start', 'app-menu-nested']),
    )
  })

  it('opens a submenu beside its entry, filters it, and relays its choice', async () => {
    const wrapper = mount(AppMenu, {
      props: {
        entries: [
          {
            kind: 'submenu',
            id: 'author',
            label: 'Author',
            testid: 'entry-author',
            panelTestid: 'author-panel',
            search: { label: 'Search authors', testid: 'author-search' },
            entries: [
              { kind: 'action', id: 'author:amy', label: 'amy', testid: 'author-amy' },
              { kind: 'action', id: 'author:ben', label: 'ben', testid: 'author-ben' },
            ],
          },
        ] satisfies MenuEntry[],
      },
      attachTo: document.body,
    })
    expect(wrapper.find('[data-testid="author-panel"]').exists()).toBe(false)
    await wrapper.get('[data-testid="entry-author"]').trigger('click')
    expect(wrapper.get('[data-testid="entry-author"]').attributes('aria-expanded')).toBe('true')
    expect(document.activeElement).toBe(wrapper.get('[data-testid="author-search"]').element)

    await wrapper.get('[data-testid="author-search"]').setValue('am')
    expect(wrapper.find('[data-testid="author-ben"]').exists()).toBe(false)
    await wrapper.get('[data-testid="author-search"]').setValue('zed')
    expect(wrapper.find('[data-testid="author-panel-empty"]').exists()).toBe(true)
    await wrapper.get('[data-testid="author-search"]').setValue('')
    await wrapper.get('[data-testid="author-ben"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([['author:ben']])

    // Escape closes the submenu alone, and the menu under it stays open.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="author-panel"]').exists()).toBe(false)
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  it('flips upward when asked to open above its anchor', () => {
    const wrapper = mount(AppMenu, { props: { entries, flip: true } })
    expect(wrapper.get('.app-menu').classes()).toContain('flip')
  })
})
