import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'

describe('publish-message editor', () => {
  it('emits a trimmed topic', async () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    await wrapper.get('[data-testid="publish-message-node-editor-topic"]').setValue(' reviews ')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ topic: 'reviews', message: '' })
  })

  it('emits the typed message template', async () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    await wrapper.get('[data-testid="publish-message-node-editor-message"]').setValue('PR {{ .Payload.num }}')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ topic: '', message: 'PR {{ .Payload.num }}' })
  })

  it('requires a literal topic and a message', () => {
    expect(validate(defaults)).toEqual(['topic is required', 'message is required'])
    expect(validate({ topic: 'agent.*', message: 'm' })).toEqual([
      'topic must be a literal without spaces, wildcards, or templates',
    ])
    expect(validate({ topic: 'agent.{{ .Key }}', message: 'm' })).toHaveLength(1)
    expect(validate({ topic: 'reviews', message: 'm' })).toEqual([])
  })
})
