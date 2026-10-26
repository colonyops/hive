import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SessionLabelChips from '../SessionLabelChips.vue'

describe('SessionLabelChips', () => {
  it('shows the first labels and counts the rest', () => {
    const wrapper = mount(SessionLabelChips, { props: { labels: ['auto', 'backend', 'review', 'urgent'] } })
    expect(wrapper.findAll('[data-testid="session-label"]').map((chip) => chip.text())).toEqual(['auto', 'backend'])
    expect(wrapper.get('[data-testid="session-labels-more"]').text()).toBe('+2')
    expect(wrapper.get('[data-testid="session-labels"]').attributes('title')).toBe('auto, backend, review, urgent')
  })

  it('renders nothing for a session without labels', () => {
    const wrapper = mount(SessionLabelChips, { props: { labels: null } })
    expect(wrapper.find('[data-testid="session-labels"]').exists()).toBe(false)
  })
})
