import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import CreateSessionDialog from '../CreateSessionDialog.vue'

const options = {
  repositories: [{ name: 'hive', repository: 'https://github.com/hay-kot/hive-desktop.git' }],
  defaultRepository: 'https://github.com/hay-kot/hive-desktop.git',
  workspaces: [{ dir: 'alerts', name: 'Alert triage', supportsPrompt: true }],
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
    expect(wrapper.emitted('submit')).toEqual([[{ name: 'review-pr-12', repository: options.defaultRepository, agent: 'claude', inputs: {} }]])
  })

  it('starts an agent workspace chat without repository or agent input', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="session-target-workspace"]').trigger('click')
    await wrapper.get('[data-testid="session-name"]').setValue('triage-alert')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ name: 'triage-alert', workspace: 'alerts', inputs: {} }]])
    expect(wrapper.find('[data-testid="session-repository"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="session-agent"]').exists()).toBe(false)
  })

  it('defaults to the first workspace that accepts an opening prompt', async () => {
    const wrapper = mountDialog({
      options: {
        ...options,
        workspaces: [
          { dir: 'plain', name: 'Plain chat', supportsPrompt: false },
          { dir: 'alerts', name: 'Alert triage', supportsPrompt: true },
        ],
      },
    })
    await wrapper.get('[data-testid="session-target-workspace"]').trigger('click')
    await wrapper.get('[data-testid="session-name"]').setValue('triage-alert')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([[{ name: 'triage-alert', workspace: 'alerts', inputs: {} }]])
  })

  it('picks a repository through the shared selector', async () => {
    const wrapper = mountDialog({
      options: { ...options, repositories: [...options.repositories, { name: 'site', repository: 'https://github.com/acme/site.git' }] },
    })
    await wrapper.get('[data-testid="session-repository"]').trigger('click')
    await wrapper.get('[data-testid="session-repository-search"]').setValue('acme')
    await wrapper.get('[data-testid="session-repository-option"]').trigger('click')
    await wrapper.get('[data-testid="session-name"]').setValue('review-pr-12')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([[{ name: 'review-pr-12', repository: 'https://github.com/acme/site.git', agent: 'claude', inputs: {} }]])
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
