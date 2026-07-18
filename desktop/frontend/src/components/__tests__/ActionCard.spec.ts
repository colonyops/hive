import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ActionCard from '../ActionCard.vue'
import type { Action } from '../../types/feed'

const baseAction: Action = {
  id: 'summarize',
  icon: 'sparkles',
  color: '#f59e0b',
  title: 'Summarize thread',
  sub: 'Generate a concise summary',
  primary: true,
}

function mountAction(overrides: Partial<Action> = {}) {
  return mount(ActionCard, {
    props: {
      action: { ...baseAction, ...overrides },
    },
  })
}

describe('ActionCard', () => {
  it('renders the primary and non-primary action affordances', () => {
    const primary = mountAction({ primary: true })
    expect(primary.find('[data-testid="primary-action"]').text()).toContain('Run')
    expect(primary.find('[data-testid="secondary-affordance"]').exists()).toBe(false)

    const secondary = mountAction({ primary: false })
    expect(secondary.find('[data-testid="primary-action"]').exists()).toBe(false)
    expect(secondary.find('[data-testid="secondary-affordance"]').exists()).toBe(true)
  })

  it('emits run when clicked', async () => {
    const wrapper = mountAction()

    await wrapper.find('button.action-card').trigger('click')

    expect(wrapper.emitted('run')).toHaveLength(1)
  })
})
