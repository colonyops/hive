import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ActionCard from '../ActionCard.vue'
import type { ActionView } from '../../types/action'

const baseAction: ActionView = {
  id: 'summarize',
  label: 'Summarize thread',
  type: 'launch-session',
  showInDetail: true, requiresSessionInput: false,
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

  it('displays persisted failed command diagnostics', () => {
    const wrapper = mount(ActionCard, {
      props: {
        action: baseAction,
        run: { commandId: 42, status: 'failed', error: 'command exited 1', stdout: 'partial output', stderr: 'bad input' },
      },
    })

    expect(wrapper.get('[data-testid="action-failure"]').text()).toContain('command exited 1')
    expect(wrapper.get('[data-testid="action-stdout"]').text()).toContain('partial output')
    expect(wrapper.get('[data-testid="action-stderr"]').text()).toContain('bad input')
  })
})
