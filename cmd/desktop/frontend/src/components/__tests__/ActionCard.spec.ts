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

function mountAction(props: { action?: Partial<ActionView>; pending?: boolean } = {}) {
  return mount(ActionCard, {
    props: {
      action: { ...baseAction, ...props.action },
      pending: props.pending,
    },
  })
}

describe('ActionCard', () => {
  it('renders the action label as a condensed row and emits run when clicked', async () => {
    const wrapper = mountAction({ action: { type: 'shell' } })

    expect(wrapper.text()).toContain('Summarize thread')
    await wrapper.get('[data-testid="action-card"]').trigger('click')
    expect(wrapper.emitted('run')).toHaveLength(1)
  })

  it('shows a pending indicator and disables the row while running', () => {
    const wrapper = mountAction({ pending: true })

    expect(wrapper.get('[data-testid="run-action"]').text()).toContain('Running')
    expect(wrapper.get('[data-testid="action-card"]').attributes('disabled')).toBeDefined()
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
