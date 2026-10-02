import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({ SessionLaunchOptions: vi.fn() }))
vi.mock(
  '../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice',
  () => ({
    SessionLaunchOptions: mocks.SessionLaunchOptions,
  }),
)

import Editor from '../editor.vue'
import { ITEM_REMOTE, defaults, repoMode, validate } from '../config'

beforeEach(() => {
  mocks.SessionLaunchOptions.mockReset()
  mocks.SessionLaunchOptions.mockResolvedValue({
    repositories: [{ name: 'hive', repository: 'https://github.com/colonyops/hive.git' }],
  })
})

const repoField = '[data-testid="launch-session-node-editor-repo"]'

describe('launch-session editor', () => {
  it('renders every launch field', () => {
    const wrapper = mount(Editor, { props: { config: { ...defaults, repo: 'x/{{ .Key }}' } } })
    for (const field of ['repo-mode', 'repo', 'agent', 'session-name', 'prompt']) {
      expect(wrapper.find(`[data-testid="launch-session-node-editor-${field}"]`).exists(), field).toBe(true)
    }
  })

  it("defaults to the item's own repository", () => {
    expect(defaults.repo).toBe(ITEM_REMOTE)
    const wrapper = mount(Editor, { props: { config: defaults } })
    expect(wrapper.find(repoField).exists()).toBe(false)
    expect(wrapper.find('[data-testid="launch-session-node-editor-repo-hint"]').exists()).toBe(true)
  })

  it('reads the mode back from the stored repo', () => {
    expect(repoMode(ITEM_REMOTE)).toBe('item')
    expect(repoMode(` ${ITEM_REMOTE} `)).toBe('item')
    expect(repoMode('https://github.com/{{ .Payload.repo }}.git')).toBe('template')
    expect(repoMode('https://github.com/colonyops/hive.git')).toBe('configured')
  })

  it('picks a fixed repository from the known checkouts', async () => {
    const config = { repo: 'https://github.com/colonyops/hive.git', prompt: 'p' }
    const wrapper = mount(Editor, { props: { config }, attachTo: document.body })
    await flushPromises()
    expect(mocks.SessionLaunchOptions).toHaveBeenCalledOnce()
    expect(wrapper.get(repoField).text()).toContain('colonyops/hive')
    wrapper.unmount()
  })

  it('switching mode rewrites the repo for the item and fixed modes and keeps it for a template', async () => {
    const config = { repo: ITEM_REMOTE, prompt: 'p' }
    const wrapper = mount(Editor, { props: { config } })
    const select = wrapper.getComponent({ name: 'SelectField' })

    select.vm.$emit('update:modelValue', 'configured')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ ...config, repo: '' })

    select.vm.$emit('update:modelValue', 'template')
    await wrapper.vm.$nextTick()
    await wrapper.get(repoField).setValue('https://github.com/{{ .Payload.repo }}.git')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({
      ...config,
      repo: 'https://github.com/{{ .Payload.repo }}.git',
    })

    select.vm.$emit('update:modelValue', 'item')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ ...config, repo: ITEM_REMOTE })
  })

  it('stays in template mode while a template without braces is typed', async () => {
    const wrapper = mount(Editor, { props: { config: { repo: '{{ .Key }}', prompt: 'p' } } })
    await wrapper.get(repoField).setValue('h')
    await wrapper.setProps({ config: wrapper.emitted('update:config')!.at(-1)![0] as typeof defaults })
    expect(wrapper.get(repoField).element.tagName).toBe('INPUT')

    await wrapper.setProps({ config: { repo: ITEM_REMOTE, prompt: 'p' } })
    expect(wrapper.find(repoField).exists()).toBe(false)
  })

  it('keeps the field usable when the repository list cannot load', async () => {
    mocks.SessionLaunchOptions.mockRejectedValue(new Error('hive unavailable'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const wrapper = mount(Editor, { props: { config: { repo: 'https://example.com/a.git', prompt: 'p' } } })
    await flushPromises()
    expect(warn).toHaveBeenCalled()
    expect(wrapper.find(repoField).exists()).toBe(true)
    warn.mockRestore()
  })

  it('clears an emptied optional field back to undefined', async () => {
    const config = { repo: 'r', prompt: 'p', agent: 'claude', sessionName: 'x' }
    const wrapper = mount(Editor, { props: { config } })
    await wrapper.get('[data-testid="launch-session-node-editor-agent"]').setValue('')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ ...config, agent: undefined })
    await wrapper.get('[data-testid="launch-session-node-editor-session-name"]').setValue('')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ ...config, sessionName: undefined })
  })

  it('requires a repo and a prompt', () => {
    expect(validate({ repo: '', prompt: '' })).toEqual(['repo is required', 'prompt is required'])
    expect(validate({ repo: 'r', prompt: 'p' })).toEqual([])
  })
})

describe('launch-session editor errors', () => {
  it('shows each validation message on the field it names', () => {
    const config = { repo: '', prompt: '' }
    const wrapper = mount(Editor, { props: { config, errors: validate(config) } })
    expect(wrapper.text()).toContain('repo is required')
    expect(wrapper.text()).toContain('prompt is required')
  })
})
