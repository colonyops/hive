import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ActionCard from '../ActionCard.vue'
import type { Action } from '../../types/feed'

const baseAction: Action = {
  id: 'summarize',
  icon: '✦',
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
    expect(mountAction({ primary: true }).text()).toContain('Run ↵')
    expect(mountAction({ primary: false }).text()).toContain('▷')
  })

  it('emits run when clicked', async () => {
    const wrapper = mountAction()

    await wrapper.find('button.action-card').trigger('click')

    expect(wrapper.emitted('run')).toHaveLength(1)
  })
})
