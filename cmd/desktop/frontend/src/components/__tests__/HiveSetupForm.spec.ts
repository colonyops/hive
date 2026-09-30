import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppSelect from '../AppSelect.vue'
import HiveSetupForm from '../HiveSetupForm.vue'

const AGENTS = [
  { name: 'claude', label: 'Claude Code', skipPermissionFlags: ['--dangerously-skip-permissions'], installed: true },
  { name: 'opencode', label: 'OpenCode', skipPermissionFlags: ['--agent', 'free-permissions-runner'], installed: false },
]

function mountForm(props: Record<string, unknown> = {}) {
  return mount(HiveSetupForm, {
    props: {
      agents: AGENTS,
      selectedAgents: new Set(['claude']),
      workspaces: [],
      defaultAgent: 'claude',
      skipPermissions: false,
      customProfiles: [],
      ...props,
    },
  })
}

describe('HiveSetupForm', () => {
  it('emits removeWorkspace for the folder whose remove button was clicked', async () => {
    const wrapper = mountForm({ workspaces: [{ path: '/home/u/code', exists: true, repos: 3 }] })

    await wrapper.get('[data-testid="hive-workspace-remove-/home/u/code"]').trigger('click')

    expect(wrapper.emitted('removeWorkspace')).toEqual([['/home/u/code']])
  })

  it('asks which agent is the default only once more than one is chosen', async () => {
    const one = mountForm()
    expect(one.find('[data-testid="hive-default-agent"]').exists()).toBe(false)

    const two = mountForm({ selectedAgents: new Set(['claude', 'opencode']) })
    expect(two.find('[data-testid="hive-default-agent"]').exists()).toBe(true)
    two.getComponent(AppSelect).vm.$emit('update:modelValue', 'opencode')
    expect(two.emitted('setDefaultAgent')).toEqual([['opencode']])
  })

  it('submits a typed path from Enter and from the Add button, and clears it once it lands', async () => {
    const wrapper = mountForm()
    const input = wrapper.get('[data-testid="hive-workspace-path"]')

    await input.setValue('  ~/code  ')
    await input.trigger('keydown.enter')
    expect(wrapper.emitted('addWorkspacePath')).toEqual([['~/code']])
    expect((input.element as HTMLInputElement).value).toBe('  ~/code  ')

    await wrapper.get('[data-testid="hive-workspace-path-add"]').trigger('click')
    expect(wrapper.emitted('addWorkspacePath')).toEqual([['~/code'], ['~/code']])

    await wrapper.setProps({ workspaces: [{ path: '~/code', exists: true, repos: 2 }] })
    expect((input.element as HTMLInputElement).value).toBe('')
  })

  it('marks the agents found on PATH and counts what each folder holds', () => {
    const wrapper = mountForm({ workspaces: [{ path: '/home/u/code', exists: true, repos: 12 }] })

    expect(wrapper.get('[data-testid="hive-agent-claude"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="hive-agent-opencode"]').attributes('aria-pressed')).toBe('false')
    expect(wrapper.find('[data-testid="hive-agent-installed-claude"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="hive-agent-installed-opencode"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="hive-workspace-list"]').text()).toContain('/home/u/code')
    expect(wrapper.get('[data-testid="hive-workspace-list"]').text()).toContain('12 repositories')
  })

  it('lists profiles it has no control for so a save is not a surprise', () => {
    const wrapper = mountForm({ customProfiles: [{ name: 'fable', command: 'claude --model fable', flags: [] }] })

    expect(wrapper.get('[data-testid="hive-custom-profiles"]').text()).toContain('fable')
    expect(wrapper.get('[data-testid="hive-custom-profiles"]').text()).toContain('kept as written')
  })

  // HIVE_DEFAULT_AGENT wins over agents.default at load, so a form that let
  // someone pick an agent without saying so would be lying to them.
  it('warns when the environment is overriding the chosen agent', () => {
    const wrapper = mountForm({ defaultAgentOverride: 'opencode' })

    const warning = wrapper.get('[data-testid="hive-default-agent-override"]').text()
    expect(warning).toContain('HIVE_DEFAULT_AGENT')
    expect(warning).toContain('opencode')
  })

  it('says so when none of the agents were found on PATH', () => {
    const none = mountForm({ agents: AGENTS.map(a => ({ ...a, installed: false })) })
    expect(none.find('[data-testid="hive-no-agents-installed"]').exists()).toBe(true)

    const some = mountForm()
    expect(some.find('[data-testid="hive-no-agents-installed"]').exists()).toBe(false)
  })
})
