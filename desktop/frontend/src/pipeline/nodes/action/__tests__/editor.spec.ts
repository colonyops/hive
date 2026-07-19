import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'

describe('action editor', () => {
  it('renders the current action ref', () => {
    const wrapper = mount(Editor, { props: { config: { action: 'review-pr' } } })
    expect(wrapper.get<HTMLInputElement>('[data-testid="action-node-editor-action"]').element.value).toBe('review-pr')
  })

  it('emits an immutable update:config on edit, without mutating the config prop', async () => {
    const config = { action: '' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="action-node-editor-action"]').element
    input.value = 'review-pr'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.action).toBe('')
    expect(wrapper.emitted('update:config')).toEqual([[{ action: 'review-pr' }]])
  })
})

describe('action validate', () => {
  it('requires a non-blank action', () => {
    expect(validate(defaults)).toEqual(['action is required'])
    expect(validate({ action: 'review-pr' })).toEqual([])
  })
})
