import { afterEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import SessionLaunchDialog from '../SessionLaunchDialog.vue'

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

afterEach(() => {
  document.body.innerHTML = ''
})

function mountDialog(props: Record<string, unknown>) {
  return mount(SessionLaunchDialog, {
    attachTo: document.body,
    props: { options, ...props },
    global: { stubs: { Teleport: true } },
  })
}

describe('SessionLaunchDialog: new session', () => {
  const mountNew = (overrides: Record<string, unknown> = {}) =>
    mountDialog({ initial: blank, withPrompt: true, busy: false, error: null, failure: null, ...overrides })

  it('puts the compact Code and Chats selector with icons in the header', () => {
    const wrapper = mountNew()
    const selector = wrapper.get('header').get('[data-testid="new-session-target"]')
    expect(selector.text()).toContain('Code')
    expect(selector.text()).toContain('Chats')
    expect(selector.findAll('svg')).toHaveLength(2)
    expect(wrapper.get('form').find('[data-testid="new-session-target"]').exists()).toBe(false)
  })

  it('prefills from the draft and emits repository, name, prompt, and agent', async () => {
    const wrapper = mountNew({
      initial: { ...blank, repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash' },
    })
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([
      [{ repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash', agent: 'claude', inputs: {} }],
    ])
  })

  it('defaults the repository to the backend default and allows an empty prompt', async () => {
    const wrapper = mountNew()
    await wrapper.get('[data-testid="new-session-name"]').setValue('standalone')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([
      [{ repository: options.defaultRepository, name: 'standalone', prompt: '', agent: 'claude', inputs: {} }],
    ])
  })

  it('warns when the prompt will be passed through a temporary file', async () => {
    const wrapper = mountNew({
      options: { ...options, promptFileThresholdBytes: 5 },
      initial: { ...blank, prompt: 'ååå' },
    })

    expect(wrapper.get('[data-testid="new-session-prompt-file-warning"]').text()).toContain(
      'write it to a temporary file',
    )

    await wrapper.get('[data-testid="new-session-prompt"]').setValue('12345')
    expect(wrapper.find('[data-testid="new-session-prompt-file-warning"]').exists()).toBe(false)
  })

  it('picks a repository through the shared selector', async () => {
    const wrapper = mountNew({
      options: {
        ...options,
        repositories: [...options.repositories, { name: 'site', repository: 'https://github.com/acme/site.git' }],
      },
    })
    await wrapper.get('[data-testid="new-session-repository"]').trigger('click')
    await wrapper.get('[data-testid="new-session-repository-search"]').setValue('acme')
    await wrapper.get('[data-testid="new-session-repository-option"]').trigger('click')
    await wrapper.get('[data-testid="new-session-name"]').setValue('fix-crash')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([
      [{ repository: 'https://github.com/acme/site.git', name: 'fix-crash', prompt: '', agent: 'claude', inputs: {} }],
    ])
  })

  it('opens directly on Chats and emits no repository or agent', async () => {
    const wrapper = mountNew({
      initial: { ...blank, name: 'incident', prompt: 'Triage this alert' },
      initialTarget: 'workspace',
    })
    expect(wrapper.get('[data-testid="new-session-target-workspace"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="new-session-repository"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="new-session-agent"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="new-session-workspace"]').text()).toContain('Alert triage')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([
      [{ workspace: 'alerts', name: 'incident', prompt: 'Triage this alert', inputs: {} }],
    ])
  })

  it('restores a workspace target and preserves the name and prompt when switching targets', async () => {
    const wrapper = mountNew({
      initial: { ...blank, workspace: 'alerts', name: 'incident', prompt: 'Triage this alert' },
    })
    expect(wrapper.get('[data-testid="new-session-target-workspace"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('[data-testid="new-session-target-repository"]').trigger('click')
    await wrapper.get('[data-testid="new-session-target-workspace"]').trigger('click')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([
      [{ workspace: 'alerts', name: 'incident', prompt: 'Triage this alert', inputs: {} }],
    ])
  })

  it('refuses a prompt for a workspace that cannot take one, even from the shortcut', async () => {
    const wrapper = mountNew({
      options: { ...options, workspaces: [{ dir: 'plain', name: 'Plain', supportsPrompt: false }] },
      initial: { ...blank, workspace: 'plain', name: 'incident', prompt: 'Triage this alert' },
    })
    expect((wrapper.get('[data-testid="new-session-submit"]').element as HTMLButtonElement).disabled).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, cancelable: true }))
    await nextTick()
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="new-session-error"]').text()).toContain('does not accept an opening prompt')
  })

  it('disables workspace submission when no workspaces exist', async () => {
    const wrapper = mountNew({ options: { ...options, workspaces: [] } })
    await wrapper.get('[data-testid="new-session-target-workspace"]').trigger('click')
    expect((wrapper.get('[data-testid="new-session-submit"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('Create a workspace in Chats')
  })

  it('submits on ⌘/Ctrl+Enter from anywhere in the dialog', async () => {
    const wrapper = mountNew({
      initial: { ...blank, repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash' },
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, cancelable: true }))
    await nextTick()

    expect(wrapper.emitted('submit')).toEqual([
      [{ repository: 'acme/site', name: 'fix-crash', prompt: 'Fix the crash', agent: 'claude', inputs: {} }],
    ])
  })

  it('ignores a bare Enter outside a field so the prompt keeps its newlines', async () => {
    const wrapper = mountNew({ initial: { ...blank, repository: 'acme/site', name: 'fix-crash', prompt: '' } })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', cancelable: true }))
    await nextTick()

    expect(wrapper.emitted('submit')).toBeUndefined()
  })

  it('advertises the submit shortcut on the button', () => {
    expect(mountNew().get('[data-testid="new-session-submit"]').text()).toContain('↵')
  })

  it('binds the footer button to the form so Enter in a field submits', () => {
    const wrapper = mountNew()
    const button = wrapper.get('[data-testid="new-session-submit"]').element as HTMLButtonElement
    expect(button.type).toBe('submit')
    expect(button.form).toBe(wrapper.get('form').element)
  })

  it('restores the agent the failed attempt used, empty included', async () => {
    const wrapper = mountNew({
      initial: { ...blank, repository: 'acme/site', name: 'fix-crash', agent: '' },
      failure,
    })
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([
      [{ repository: 'acme/site', name: 'fix-crash', prompt: '', inputs: {} }],
    ])
  })

  it('shows the failing step, the reason, and the output tail', () => {
    const wrapper = mountNew({ initial: { ...blank, repository: 'acme/site', name: 'fix-crash' }, failure })
    expect(wrapper.get('[data-testid="new-session-failure-reason"]').text()).toContain('Cloning repository...')
    expect(wrapper.get('[data-testid="new-session-failure-reason"]').text()).toContain('exit status 1')
    expect(wrapper.get('[data-testid="new-session-failure-output"]').text()).toContain('Clone strategy: full')
    expect(wrapper.get('[data-testid="new-session-submit"]').text()).toContain('Try again')
  })

  it('names the checkout a failure left behind', () => {
    const wrapper = mountNew({ failure })
    expect(wrapper.get('[data-testid="new-session-failure"]').text()).toContain(
      '/home/u/.local/share/hive/repos/site-9fa2',
    )
  })

  // git removes its own directory when it refuses a clone, so hive names a
  // destination that is already gone. Claiming it is on disk would be a lie.
  it('says nothing about a checkout git already removed', () => {
    const wrapper = mountNew({ failure: { ...failure, leftoverCheckout: false } })
    expect(wrapper.get('[data-testid="new-session-failure"]').text()).not.toContain('safe to delete')
  })

  it('has no failure panel on a fresh form', () => {
    expect(mountNew().find('[data-testid="new-session-failure"]').exists()).toBe(false)
  })

  it('emits dismissFailure without closing the form', async () => {
    const wrapper = mountNew({ failure })
    await wrapper.get('[data-testid="new-session-failure-dismiss"]').trigger('click')
    expect(wrapper.emitted('dismissFailure')).toHaveLength(1)
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('stays open on a backdrop click and closes only from the close button', async () => {
    const wrapper = mountNew()
    await wrapper.get('[data-testid="new-session-dialog-backdrop"]').trigger('click')
    expect(wrapper.emitted('close')).toBeUndefined()
    await wrapper.get('[data-testid="new-session-dialog-close"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('keeps the dialog open and reports invalid names locally', async () => {
    const wrapper = mountNew()
    await wrapper.get('[data-testid="new-session-name"]').setValue('bad@name')
    await wrapper.get('[data-testid="new-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="new-session-error"]').text()).toContain('Use letters')
  })
})

describe('SessionLaunchDialog: launched from an action', () => {
  const mountAction = (overrides: Record<string, unknown> = {}) =>
    mountDialog({
      action: { label: 'Review' },
      initial: { agent: options.defaultAgent },
      busy: false,
      error: null,
      testid: 'create-session',
      fieldTestid: 'session',
      ...overrides,
    })

  it('titles the dialog with the action and has no prompt field', () => {
    const wrapper = mountAction()
    expect(wrapper.get('[data-testid="create-session-dialog"]').text()).toContain('Review')
    expect(wrapper.find('[data-testid="session-prompt"]').exists()).toBe(false)
  })

  it('uses backend defaults and emits validated session input', async () => {
    const wrapper = mountAction()
    await wrapper.get('[data-testid="session-name"]').setValue('review-pr-12')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([
      [{ name: 'review-pr-12', repository: options.defaultRepository, agent: 'claude', inputs: {} }],
    ])
  })

  it('starts an agent workspace chat without repository or agent input', async () => {
    const wrapper = mountAction()
    await wrapper.get('[data-testid="session-target-workspace"]').trigger('click')
    await wrapper.get('[data-testid="session-name"]').setValue('triage-alert')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ name: 'triage-alert', workspace: 'alerts', inputs: {} }]])
    expect(wrapper.find('[data-testid="session-repository"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="session-agent"]').exists()).toBe(false)
  })

  it('defaults to the first workspace that accepts an opening prompt', async () => {
    const wrapper = mountAction({
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

  it('requires a prompt-capable workspace', async () => {
    const wrapper = mountAction({
      options: { ...options, workspaces: [{ dir: 'plain', name: 'Plain chat', supportsPrompt: false }] },
    })
    await wrapper.get('[data-testid="session-target-workspace"]').trigger('click')
    await wrapper.get('[data-testid="session-name"]').setValue('triage-alert')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="create-session-error"]').text()).toContain('prompt-capable')
  })

  it('collects and validates the action inputs', async () => {
    const wrapper = mountAction({
      action: {
        label: 'Review',
        inputs: [{ name: 'focus', label: 'Focus', type: 'text', required: true, default: '' }],
      },
    })
    await wrapper.get('[data-testid="session-name"]').setValue('review-pr-12')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="create-session-error"]').text()).toContain('Focus is required')
  })

  it('keeps the dialog open and reports invalid input locally', async () => {
    const wrapper = mountAction()
    await wrapper.get('[data-testid="session-name"]').setValue('bad@name')
    await wrapper.get('[data-testid="create-session-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="create-session-error"]').text()).toContain('Use letters')
  })

  it('does not cancel while creating', async () => {
    const wrapper = mountAction({ busy: true })
    await wrapper.get('button[aria-label="Close"]').trigger('click')
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})

describe('SessionLaunchDialog: new chat', () => {
  const workspaces = [
    { dir: 'web-app', name: 'Web App' },
    { dir: 'api', name: 'API' },
  ]
  const mountChat = (overrides: Record<string, unknown> = {}) =>
    mountDialog({
      options: { workspaces },
      initial: { workspace: 'web-app' },
      workspaceOnly: true,
      defaultName: 'New Chat',
      noWorkspacesHint: 'No workspaces yet. Author one under /tmp/agents.',
      testid: 'new-chat',
      ...overrides,
    })

  it('has no target selector', () => {
    expect(mountChat().find('[data-testid="new-chat-target"]').exists()).toBe(false)
  })

  it('opens on the given workspace and emits a trimmed name', async () => {
    const wrapper = mountChat()
    await wrapper.get('[data-testid="new-chat-name"]').setValue('  Ship it  ')
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ workspace: 'web-app', name: 'Ship it', inputs: {} }]])
  })

  it('submits an empty name as the default name, unvalidated', async () => {
    const wrapper = mountChat()
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')
    await wrapper.get('[data-testid="new-chat-name"]').setValue('Fix #12!')
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([
      [{ workspace: 'web-app', name: 'New Chat', inputs: {} }],
      [{ workspace: 'web-app', name: 'Fix #12!', inputs: {} }],
    ])
  })

  it('starts the chat in another workspace when one is picked', async () => {
    const wrapper = mountChat()
    await wrapper.get('[data-testid="new-chat-workspace"]').trigger('click')
    await wrapper.vm.$nextTick()
    document.querySelector<HTMLElement>('[data-testid="new-chat-workspace-option-api"]')!.click()
    await flushPromises()
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ workspace: 'api', name: 'New Chat', inputs: {} }]])
  })

  it('names the workspace root when there are no workspaces', async () => {
    const wrapper = mountChat({ options: { workspaces: [] }, initial: {} })
    expect(wrapper.get('[data-testid="new-chat-workspace-hint"]').text()).toContain('/tmp/agents')
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toBeUndefined()
  })
})
