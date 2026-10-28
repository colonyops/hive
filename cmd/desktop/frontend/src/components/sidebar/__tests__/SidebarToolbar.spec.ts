import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SidebarToolbar from '../SidebarToolbar.vue'
import type { MenuEntry } from '@hive/ui/types/menu'

const entries: MenuEntry[] = [{ kind: 'action', id: 'expand-all', label: 'Expand all', testid: 'entry-expand' }]

function mountToolbar(props: Record<string, unknown> = {}, attrs: Record<string, unknown> = {}) {
  return mount(SidebarToolbar, {
    props: {
      modelValue: '',
      filterLabel: 'Filter',
      reloadLabel: 'Reload',
      newLabel: 'New',
      menuLabel: 'List actions',
      menuEntries: entries,
      testid: 'tree',
      reloadTestid: 'tree-reload',
      newTestid: 'tree-new',
      ...props,
    },
    attrs,
    attachTo: document.body,
  })
}

describe('SidebarToolbar', () => {
  it('derives the filter and menu ids and emits reload and new', async () => {
    const wrapper = mountToolbar()
    await wrapper.get('[data-testid="tree-filter"]').setValue('abc')
    expect(wrapper.emitted('update:modelValue')).toEqual([['abc']])
    await wrapper.get('[data-testid="tree-reload"]').trigger('click')
    await wrapper.get('[data-testid="tree-new"]').trigger('click')
    expect(wrapper.emitted('reload')).toHaveLength(1)
    expect(wrapper.emitted('new')).toHaveLength(1)
    wrapper.unmount()
  })

  it('disables reload while loading and new while creating', () => {
    const wrapper = mountToolbar({ loading: true, creating: true })
    expect(wrapper.get('[data-testid="tree-reload"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="tree-new"]').attributes('aria-busy')).toBe('true')
    wrapper.unmount()
  })

  it('opens the list menu, relays a choice, and closes', async () => {
    const wrapper = mountToolbar()
    await wrapper.get('[data-testid="tree-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="entry-expand"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([['expand-all']])
    expect(wrapper.find('[data-testid="tree-menu"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('hands its listeners to the filter, not to the buttons', async () => {
    const onKeydown = vi.fn()
    const wrapper = mountToolbar({}, { onKeydown })
    await wrapper.get('[data-testid="tree-reload"]').trigger('keydown', { key: 'Enter' })
    expect(onKeydown).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="tree-filter"]').trigger('keydown', { key: 'ArrowDown' })
    expect(onKeydown).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('leaves reload out when the host keeps it elsewhere, and spins while loading', () => {
    const wrapper = mount(SidebarToolbar, {
      props: {
        modelValue: '',
        filterLabel: 'Filter',
        newLabel: 'New',
        menuLabel: 'List actions',
        menuEntries: entries,
        testid: 'tree',
        newTestid: 'tree-new',
        loading: true,
      },
      slots: { default: '<button data-testid="extra">x</button>' },
    })
    expect(wrapper.find('[data-testid="tree-reload"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="tree-reloading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="extra"]').exists()).toBe(true)
  })
})
