import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ActionCard from '../ActionCard.vue'
import type { ActionView } from '../../types/action'

const baseAction: ActionView = {
  id: 'summarize',
  label: 'Summarize thread',
  type: 'launch-session',
  showInDetail: true,
}

function mountAction(overrides: Partial<ActionView> = {}) {
  return mount(ActionCard, {
    props: {
      action: { ...baseAction, ...overrides },
    },
  })
}

describe('ActionCard', () => {
  it('derives presentation from the configured action type', () => {
    const wrapper = mountAction({ type: 'shell' })

    expect(wrapper.text()).toContain('Summarize thread')
    expect(wrapper.text()).toContain('Run shell command')
    expect(wrapper.find('[data-testid="run-action"]').text()).toContain('Run')
  })

  it('emits run when clicked', async () => {
    const wrapper = mountAction()

    await wrapper.find('button.action-card').trigger('click')

    expect(wrapper.emitted('run')).toHaveLength(1)
  })
})
