import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'
import { resetStores } from '../../../../stores/defineStore'

const mocks = vi.hoisted(() => ({ Available: vi.fn(), workspaces: vi.fn() }))

vi.mock('../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
}))
vi.mock('../../../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: vi.fn().mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' }),
  createAgentWorkspacesClient: () => ({ workspaces: mocks.workspaces }),
}))

function payload(dirs: string[]) {
  return {
    root: '/root',
    rootProblem: '',
    available: true,
    error: '',
    editor: { command: '', title: '' },
    presets: [],
    workspaces: dirs.map((dir) => ({
      dir,
      name: `${dir} workspace`,
      command: 'claude',
      danger: false,
      mcps: [],
      skills: [],
      schedules: [],
      problem: '',
      notice: '',
    })),
  }
}

describe('launch-chat editor', () => {
  beforeEach(() => {
    resetStores()
    vi.clearAllMocks()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.workspaces.mockResolvedValue(payload(['incident-triage']))
  })

  it('offers the known workspaces and keeps a stored one that is gone', async () => {
    const wrapper = mount(Editor, { props: { config: { workspace: 'retired', prompt: 'p' } } })
    await flushPromises()
    const field = wrapper.get('[data-testid="launch-chat-node-editor-workspace"]')
    expect(field.text()).toContain('retired (not found)')
  })

  it('falls back to a text field when the Agents area is unavailable', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'no pty' })
    const wrapper = mount(Editor, { props: { config: defaults } })
    await flushPromises()
    await wrapper.get('input[data-testid="launch-chat-node-editor-workspace"]').setValue(' triage ')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ workspace: 'triage', prompt: '' })
  })

  it('emits the typed prompt template', async () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    await wrapper.get('[data-testid="launch-chat-node-editor-prompt"]').setValue('Triage {{ .Payload.title }}')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({
      workspace: '',
      prompt: 'Triage {{ .Payload.title }}',
    })
  })

  it('requires a workspace and a prompt', () => {
    expect(validate(defaults)).toEqual(['workspace is required', 'prompt is required'])
    expect(validate({ workspace: 'w', prompt: 'p' })).toEqual([])
  })
})
