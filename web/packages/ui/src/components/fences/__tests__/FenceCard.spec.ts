import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { fakePlatform } from '../../../test-utils/platform'
import FenceCard from '../FenceCard.vue'

const mountCard = () =>
  mount(FenceCard, {
    props: { kind: 'Files', title: '3 files', summary: '+1', source: 'files: []\n', lang: 'hive-tree', testid: 'c' },
    slots: { default: '<p class="body">rows</p>' },
  })

describe('FenceCard', () => {
  it('shows the kind, title, summary and body', () => {
    const wrapper = mountCard()
    expect(wrapper.text()).toContain('Files')
    expect(wrapper.text()).toContain('3 files')
    expect(wrapper.text()).toContain('+1')
    expect(wrapper.find('.body').exists()).toBe(true)
  })

  it('toggles the raw source in place of the body', async () => {
    const wrapper = mountCard()
    await wrapper.get('[data-testid="c-source-toggle"]').trigger('click')
    expect(wrapper.find('.body').exists()).toBe(false)
    expect(wrapper.get('[data-testid="c-source"]').text()).toBe('```hive-tree\nfiles: []\n```')
    await wrapper.get('[data-testid="c-source-toggle"]').trigger('click')
    expect(wrapper.find('.body').exists()).toBe(true)
  })

  it('copies the raw body', async () => {
    const wrapper = mountCard()
    await wrapper.get('[data-testid="c-copy"]').trigger('click')
    await flushPromises()
    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('files: []\n')
  })

  it('collapses the body and emits the model update', async () => {
    const wrapper = mountCard()
    await wrapper.get('[data-testid="c-collapse"]').trigger('click')
    expect(wrapper.find('.body').exists()).toBe(false)
    expect(wrapper.get('[data-testid="c-collapse"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.emitted('update:collapsed')).toEqual([[true]])
  })

  it('hides Collapse when not collapsible', () => {
    const wrapper = mount(FenceCard, { props: { kind: 'x', source: '', collapsible: false, testid: 'c' } })
    expect(wrapper.find('[data-testid="c-collapse"]').exists()).toBe(false)
  })
})
