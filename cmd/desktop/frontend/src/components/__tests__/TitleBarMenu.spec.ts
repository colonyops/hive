import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TitleBarMenu from '../TitleBarMenu.vue'

describe('TitleBarMenu', () => {
  it('runs the command an entry names and closes', async () => {
    const wrapper = mount(TitleBarMenu, { attachTo: document.body })
    await wrapper.get('[data-testid="titlebar-menu"]').trigger('click')
    expect(wrapper.text()).toContain('Action runs')

    await wrapper.get('[data-testid="titlebar-menu-action-runs"]').trigger('click')
    expect(wrapper.emitted('run-command')).toEqual([['action-runs.toggle']])
    expect(wrapper.find('[data-testid="titlebar-menu-panel"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('lists application settings after the views', async () => {
    const wrapper = mount(TitleBarMenu, { attachTo: document.body })
    await wrapper.get('[data-testid="titlebar-menu"]').trigger('click')

    await wrapper.get('[data-testid="application-settings"]').trigger('click')
    expect(wrapper.emitted('run-command')).toEqual([['settings.open']])
    wrapper.unmount()
  })
})
