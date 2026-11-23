import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'

describe('create-task editor', () => {
  it('emits the typed title template', async () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    await wrapper.get('[data-testid="create-task-node-editor-title"]').setValue('Triage {{ .Payload.title }}')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'Triage {{ .Payload.title }}' })
  })

  it('drops an optional field that is cleared', async () => {
    const wrapper = mount(Editor, { props: { config: { title: 't', repo: 'acme/api' } } })
    await wrapper.get('[data-testid="create-task-node-editor-repo"]').setValue('  ')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 't', repo: undefined })
  })

  it('trims the parent id', async () => {
    const wrapper = mount(Editor, { props: { config: { title: 't' } } })
    await wrapper.get('[data-testid="create-task-node-editor-parent"]').setValue(' hc-a1 ')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 't', parent: 'hc-a1' })
  })

  it('requires a title and a literal parent', () => {
    expect(validate(defaults)).toEqual(['title is required'])
    expect(validate({ title: 't', parent: '{{ .Key }}' })).toEqual(['parent must be a task id'])
    expect(validate({ title: 't', parent: 'hc-a1', repo: '{{ .Payload.repo }}' })).toEqual([])
  })
})
