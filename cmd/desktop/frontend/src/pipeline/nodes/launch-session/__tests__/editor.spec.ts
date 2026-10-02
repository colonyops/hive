import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'

describe('launch-session editor', () => {
  it('renders every launch field', () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    for (const field of ['repo', 'agent', 'session-name', 'prompt']) {
      expect(wrapper.find(`[data-testid="launch-session-node-editor-${field}"]`).exists(), field).toBe(true)
    }
  })

  it('emits the typed repo and prompt templates', async () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    await wrapper.get('[data-testid="launch-session-node-editor-repo"]').setValue('{{ .Payload.repo }}')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ repo: '{{ .Payload.repo }}', prompt: '' })
    await wrapper.get('[data-testid="launch-session-node-editor-prompt"]').setValue('Review it')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ repo: '', prompt: 'Review it' })
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
    expect(validate(defaults)).toEqual(['repo is required', 'prompt is required'])
    expect(validate({ repo: 'r', prompt: 'p' })).toEqual([])
  })
})

describe('launch-session editor errors', () => {
  it('shows each validation message on the field it names', () => {
    const wrapper = mount(Editor, { props: { config: defaults, errors: validate(defaults) } })
    expect(wrapper.text()).toContain('repo is required')
    expect(wrapper.text()).toContain('prompt is required')
  })
})
