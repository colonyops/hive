import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import CreateSessionDialog from '../CreateSessionDialog.vue'

const options = {
  repositories: [{ name: 'hive', repository: 'https://github.com/colonyops/hive.git' }],
  defaultRepository: 'https://github.com/colonyops/hive.git',
  agents: ['claude', 'pi'],
  defaultAgent: 'claude',
}

function mountDialog(overrides: Record<string, unknown> = {}) {
  return mount(CreateSessionDialog, { attachTo: document.body, props: { actionLabel: 'Review', options, busy: false, error: null, ...overrides }, global: { stubs: { Teleport: true } } })
}

describe('CreateSessionDialog', () => {
  it('uses backend defaults and emits validated session input', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="session-name"]').setValue('review-pr-12')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ name: 'review-pr-12', repository: options.defaultRepository, agent: 'claude' }]])
  })

  it('keeps the dialog open and reports invalid input locally', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="session-name"]').setValue('bad@name')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="create-session-error"]').text()).toContain('Use letters')
  })

  it('does not cancel while creating', async () => {
    const wrapper = mountDialog({ busy: true })
    await wrapper.get('button[aria-label="Close"]').trigger('click')
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})
