import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import DevBar from '../DevBar.vue'
import { createAppRouter } from '../../router'

describe('DevBar', () => {
  it('offers one link to the developer tools page without inline controls', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/feed')
    await router.isReady()
    const wrapper = mount(DevBar, { global: { plugins: [router] } })

    expect(wrapper.findAll('a')).toHaveLength(1)
    expect(wrapper.find('[data-testid="devbar-link"]').text()).toContain('Dev tools')
    expect(wrapper.findAll('button, select, input')).toHaveLength(0)
    expect(wrapper.find('[data-testid="devbar-notification-test"]').exists()).toBe(false)

    await wrapper.find('[data-testid="devbar-link"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/dev')

    wrapper.unmount()
  })
})
