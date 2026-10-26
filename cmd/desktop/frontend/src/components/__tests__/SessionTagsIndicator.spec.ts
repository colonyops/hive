import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SessionTagsIndicator from '../SessionTagsIndicator.vue'
import SessionTagsStatus from '../SessionTagsStatus.vue'

describe('SessionTagsIndicator', () => {
  it('names every tag for assistive tech and switches to the multiple-tags icon', () => {
    const wrapper = mount(SessionTagsIndicator, { props: { tags: ['auto', 'backend', 'review'] } })
    const indicator = wrapper.get('[data-testid="session-tags"]')
    expect(indicator.attributes('aria-label')).toBe('Tags: auto, backend, review')
    expect(indicator.attributes('data-count')).toBe('many')
    expect(indicator.text()).toBe('')
  })

  it('shows the single-tag icon for one tag', () => {
    const wrapper = mount(SessionTagsIndicator, { props: { tags: ['auto'] } })
    expect(wrapper.get('[data-testid="session-tags"]').attributes('data-count')).toBe('one')
  })

  it('renders nothing for a session without tags', () => {
    const wrapper = mount(SessionTagsIndicator, { props: { tags: null } })
    expect(wrapper.find('[data-testid="session-tags"]').exists()).toBe(false)
  })
})

describe('SessionTagsStatus', () => {
  it('lists the tags and asks for the editor on click', async () => {
    const wrapper = mount(SessionTagsStatus, { props: { tags: ['auto', 'backend'] } })
    const button = wrapper.get('[data-testid="session-status-tags"]')
    expect(button.text()).toBe('auto, backend')
    await button.trigger('click')
    expect(wrapper.emitted('edit')).toHaveLength(1)
  })

  it('renders nothing without tags', () => {
    const wrapper = mount(SessionTagsStatus, { props: { tags: [] } })
    expect(wrapper.find('[data-testid="session-status-tags"]').exists()).toBe(false)
  })
})
