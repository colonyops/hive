import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import NewChatDialog from '../NewChatDialog.vue'

const workspaces = [
  { dir: 'web-app', name: 'Web App', command: 'claude', danger: false, mcps: [], skills: [], schedules: [], problem: '', notice: '' },
  { dir: 'api', name: 'API', command: 'claude', danger: false, mcps: [], skills: [], schedules: [], problem: '', notice: '' },
]

function mountDialog(overrides: Record<string, unknown> = {}) {
  return mount(NewChatDialog, {
    attachTo: document.body,
    props: { workspaces, initialWorkspace: 'web-app', root: '/tmp/agents', ...overrides },
    global: { stubs: { Teleport: true } },
  })
}

describe('NewChatDialog', () => {
  afterEach(() => { document.body.innerHTML = '' })

  it('opens on the given workspace and emits a trimmed name', async () => {
    const wrapper = mountDialog()

    await wrapper.get('[data-testid="new-chat-name"]').setValue('  Ship it  ')
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([[{ workspace: 'web-app', name: 'Ship it' }]])
  })

  // The name is optional — AgentsMode applies the default, so an empty one
  // must reach it as empty rather than being blocked here.
  it('submits with no name', async () => {
    const wrapper = mountDialog()

    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([[{ workspace: 'web-app', name: '' }]])
  })

  it('starts the chat in another workspace when one is picked', async () => {
    const wrapper = mountDialog()

    // AppSelect teleports its popover to document.body, so the option is not
    // inside the wrapper.
    await wrapper.get('[data-testid="new-chat-workspace"]').trigger('click')
    await wrapper.vm.$nextTick()
    document.querySelector<HTMLElement>('[data-testid="new-chat-workspace-option-api"]')!.click()
    await flushPromises()
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([[{ workspace: 'api', name: '' }]])
  })

  // Nothing can be started without a workspace, so the dialog says where they
  // live instead of offering a submit that would fail.
  it('names the workspace root when there are no workspaces', async () => {
    const wrapper = mountDialog({ workspaces: [], initialWorkspace: '' })

    expect(wrapper.get('[data-testid="new-chat-no-workspaces"]').text()).toContain('/tmp/agents')
    await wrapper.get('[data-testid="new-chat-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toBeUndefined()
  })
})
