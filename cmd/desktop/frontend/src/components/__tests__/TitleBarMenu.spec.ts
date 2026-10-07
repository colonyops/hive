import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TitleBarMenu from '../TitleBarMenu.vue'

describe('TitleBarMenu', () => {
  it('runs the command an entry names and closes', async () => {
    const wrapper = mount(TitleBarMenu, { attachTo: document.body })
    await wrapper.get('[data-testid="titlebar-menu"]').trigger('click')
    expect(wrapper.text()).toContain('Action runs')
    expect(wrapper.text()).not.toContain('Settings')

    await wrapper.get('[data-testid="titlebar-menu-action-runs"]').trigger('click')
    expect(wrapper.emitted('run-command')).toEqual([['action-runs.toggle']])
    expect(wrapper.find('[data-testid="titlebar-menu-panel"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
