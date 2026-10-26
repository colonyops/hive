import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SessionLabelsIndicator from '../SessionLabelsIndicator.vue'
import SessionLabelsStatus from '../SessionLabelsStatus.vue'

describe('SessionLabelsIndicator', () => {
  it('names every label for assistive tech and counts them when there are several', () => {
    const wrapper = mount(SessionLabelsIndicator, { props: { labels: ['auto', 'backend', 'review'] } })
    const indicator = wrapper.get('[data-testid="session-labels"]')
    expect(indicator.attributes('aria-label')).toBe('Labels: auto, backend, review')
    expect(indicator.text()).toBe('3')
  })

  it('shows only the icon for a single label', () => {
    const wrapper = mount(SessionLabelsIndicator, { props: { labels: ['auto'] } })
    expect(wrapper.get('[data-testid="session-labels"]').text()).toBe('')
  })

  it('renders nothing for a session without labels', () => {
    const wrapper = mount(SessionLabelsIndicator, { props: { labels: null } })
    expect(wrapper.find('[data-testid="session-labels"]').exists()).toBe(false)
  })
})

describe('SessionLabelsStatus', () => {
  it('lists the labels and asks for the editor on click', async () => {
    const wrapper = mount(SessionLabelsStatus, { props: { labels: ['auto', 'backend'] } })
    const button = wrapper.get('[data-testid="session-status-labels"]')
    expect(button.text()).toBe('auto, backend')
    await button.trigger('click')
    expect(wrapper.emitted('edit')).toHaveLength(1)
  })

  it('renders nothing without labels', () => {
    const wrapper = mount(SessionLabelsStatus, { props: { labels: [] } })
    expect(wrapper.find('[data-testid="session-status-labels"]').exists()).toBe(false)
  })
})
