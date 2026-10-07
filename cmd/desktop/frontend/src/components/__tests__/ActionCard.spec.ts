import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ActionCard from '../ActionCard.vue'
import type { ActionView } from '../../types/action'

const baseAction: ActionView = {
  id: 'summarize',
  label: 'Summarize thread',
  type: 'launch-session',
  showInDetail: true,
  requiresSessionInput: false,
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

  it('shows a starting indicator while the invocation is accepted', () => {
    const wrapper = mountAction({ pending: true })

    expect(wrapper.get('[data-testid="run-action"]').text()).toContain('Starting')
    expect(wrapper.get('[data-testid="action-card"]').attributes('disabled')).toBeDefined()
  })

  it('shows a running command without rendering it as a failure', () => {
    const wrapper = mount(ActionCard, {
      props: { action: baseAction, run: { commandId: 42, status: 'running' } },
    })

    expect(wrapper.get('[data-testid="run-action"]').text()).toContain('Running')
    expect(wrapper.find('[data-testid="action-failure"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="action-card"]').attributes('disabled')).toBeDefined()
  })

  it('links a finished run to its log without an outcome line', async () => {
    const wrapper = mount(ActionCard, {
      props: { action: baseAction, run: { commandId: 42, status: 'done', stdout: 'finished quickly' } },
    })

    expect(wrapper.find('[data-testid="action-failure"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('finished quickly')
    const link = wrapper.get('[data-testid="action-open-log"]')
    expect(link.text()).toBe('View log')
    await link.trigger('click')
    expect(wrapper.emitted('open-log')).toEqual([[42]])
  })

  it('shows the failure reason beside the log link', () => {
    const wrapper = mount(ActionCard, {
      props: {
        action: baseAction,
        run: { commandId: 42, status: 'failed', error: 'command exited 1', stderr: 'bad input' },
      },
    })

    expect(wrapper.get('[data-testid="action-failure"]').text()).toBe('command exited 1')
    expect(wrapper.text()).not.toContain('bad input')
    expect(wrapper.get('[data-testid="action-open-log"]').text()).toBe('View log')
  })

  it('offers the live log while a run is in flight', () => {
    const wrapper = mount(ActionCard, { props: { action: baseAction, run: { commandId: 42, status: 'running' } } })

    expect(wrapper.get('[data-testid="action-open-log"]').text()).toBe('View live log')
  })
})
