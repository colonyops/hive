import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import NewSessionDialog from '../NewSessionDialog.vue'

const options = {
  repositories: [{ name: 'hive', repository: 'https://github.com/hay-kot/hive-desktop.git' }],
  defaultRepository: 'https://github.com/hay-kot/hive-desktop.git',
  workspaces: [{ dir: 'alerts', name: 'Alert triage', supportsPrompt: true }],
  agents: ['claude', 'pi'],
  defaultAgent: 'claude',
}

const blank = { repository: '', workspace: '', name: '', prompt: '', agent: options.defaultAgent }

const failure = {
  reason: 'clone repository: git clone: exec git: exit status 1',
  step: 'Cloning repository...',
  output: 'Clone strategy: full\nCloning repository...',
  cloneStrategy: 'full',
  destination: '/home/u/.local/share/hive/repos/site-9fa2',
  leftoverCheckout: true,
  at: '2026-09-16T10:00:00Z',
}

function mountDialog(overrides: Record<string, unknown> = {}) {
  return mount(NewSessionDialog, { attachTo: document.body, props: { options, initial: blank, busy: false, error: null, failure: null, ...overrides }, global: { stubs: { Teleport: true } } })
}

describe('NewSessionDialog', () => {
  it('puts the compact Code and Chats selector with icons in the header', () => {
    const wrapper = mountDialog()
    const selector = wrapper.get('header').get('[data-testid="new-session-target"]')
    expect(selector.text()).toContain('Code')
    expect(selector.text()).toContain('Chats')
    expect(selector.findAll('svg')).toHaveLength(2)
    expect(wrapper.get('form').find('[data-testid="new-session-target"]').exists()).toBe(false)
  })

  it('prefills from the draft and emits repository, name, prompt, and agent', async () => {
    const wrapper = mountDialog({ initial: { ...blank, repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash' } })
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash', agent: 'claude' }]])
  })

  it('defaults the repository to the backend default and allows an empty prompt', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="new-session-name"]').setValue('standalone')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ repository: options.defaultRepository, name: 'standalone', prompt: '', agent: 'claude' }]])
  })

  it('picks a repository through the shared selector', async () => {
    const wrapper = mountDialog({
      options: { ...options, repositories: [...options.repositories, { name: 'site', repository: 'https://github.com/acme/site.git' }] },
    })
    await wrapper.get('[data-testid="new-session-repository"]').trigger('click')
    await wrapper.get('[data-testid="new-session-repository-search"]').setValue('acme')
    await wrapper.get('[data-testid="new-session-repository-option"]').trigger('click')
    await wrapper.get('[data-testid="new-session-name"]').setValue('fix-crash')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([[{ repository: 'https://github.com/acme/site.git', name: 'fix-crash', prompt: '', agent: 'claude' }]])
  })

  it('opens directly on Chats and emits no repository or agent', async () => {
    const wrapper = mountDialog({ initial: { ...blank, name: 'incident', prompt: 'Triage this alert' }, initialTarget: 'workspace' })
    expect(wrapper.get('[data-testid="new-session-target-workspace"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="new-session-repository"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="new-session-agent"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="new-session-workspace"]').text()).toContain('Alert triage')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ workspace: 'alerts', name: 'incident', prompt: 'Triage this alert' }]])
  })

  it('restores a workspace target and preserves the name and prompt when switching targets', async () => {
    const wrapper = mountDialog({ initial: { ...blank, workspace: 'alerts', name: 'incident', prompt: 'Triage this alert' } })
    expect(wrapper.get('[data-testid="new-session-target-workspace"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('[data-testid="new-session-target-repository"]').trigger('click')
    await wrapper.get('[data-testid="new-session-target-workspace"]').trigger('click')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ workspace: 'alerts', name: 'incident', prompt: 'Triage this alert' }]])
  })

  it('disables workspace submission when no workspaces exist', async () => {
    const wrapper = mountDialog({ options: { ...options, workspaces: [] } })
    await wrapper.get('[data-testid="new-session-target-workspace"]').trigger('click')
    expect((wrapper.get('[data-testid="new-session-submit"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('Create a workspace in Chats')
  })

  it('submits on ⌘/Ctrl+Enter from anywhere in the dialog', async () => {
    const wrapper = mountDialog({ initial: { ...blank, repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash' } })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, cancelable: true }))
    await nextTick()

    expect(wrapper.emitted('submit')).toEqual([[{ repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash', agent: 'claude' }]])
  })

  it('ignores a bare Enter outside a field so the prompt keeps its newlines', async () => {
    const wrapper = mountDialog({ initial: { ...blank, repository: 'acme/site', name: 'fix-crash', prompt: '' } })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', cancelable: true }))
    await nextTick()

    expect(wrapper.emitted('submit')).toBeUndefined()
  })

  it('advertises the submit shortcut on the button', () => {
    expect(mountDialog().get('[data-testid="new-session-submit"]').text()).toContain('↵')
  })

  it('binds the footer button to the form so Enter in a field submits', () => {
    const wrapper = mountDialog()
    const button = wrapper.get('[data-testid="new-session-submit"]').element as HTMLButtonElement
    expect(button.type).toBe('submit')
    expect(button.form).toBe(wrapper.get('form').element)
  })

  it('restores the agent the failed attempt used, empty included', async () => {
    const wrapper = mountDialog({ initial: { ...blank, repository: 'acme/site', name: 'fix-crash', agent: '' }, failure })
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ repository: 'acme/site', name: 'fix-crash', prompt: '' }]])
  })

  it('shows the failing step, the reason, and the output tail', () => {
    const wrapper = mountDialog({ initial: { ...blank, repository: 'acme/site', name: 'fix-crash' }, failure })
    expect(wrapper.get('[data-testid="new-session-failure-reason"]').text()).toContain('Cloning repository...')
    expect(wrapper.get('[data-testid="new-session-failure-reason"]').text()).toContain('exit status 1')
    expect(wrapper.get('[data-testid="new-session-failure-output"]').text()).toContain('Clone strategy: full')
    expect(wrapper.get('[data-testid="new-session-submit"]').text()).toContain('Try again')
  })

  it('names the checkout a failure left behind', () => {
    const wrapper = mountDialog({ failure })
    expect(wrapper.get('[data-testid="new-session-failure"]').text()).toContain('/home/u/.local/share/hive/repos/site-9fa2')
  })

  // git removes its own directory when it refuses a clone, so hive names a
  // destination that is already gone. Claiming it is on disk would be a lie.
  it('says nothing about a checkout git already removed', () => {
    const wrapper = mountDialog({ failure: { ...failure, leftoverCheckout: false } })
    expect(wrapper.get('[data-testid="new-session-failure"]').text()).not.toContain('safe to delete')
  })

  it('has no failure panel on a fresh form', () => {
    expect(mountDialog().find('[data-testid="new-session-failure"]').exists()).toBe(false)
  })

  it('emits dismissFailure without closing the form', async () => {
    const wrapper = mountDialog({ failure })
    await wrapper.get('[data-testid="new-session-failure-dismiss"]').trigger('click')
    expect(wrapper.emitted('dismissFailure')).toHaveLength(1)
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('keeps the dialog open and reports invalid names locally', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="new-session-name"]').setValue('bad@name')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="new-session-error"]').text()).toContain('Use letters')
  })
})
