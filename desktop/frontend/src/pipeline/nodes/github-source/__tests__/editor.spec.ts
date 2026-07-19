import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'

describe('github-source editor', () => {
  it('renders the current source ref', () => {
    const wrapper = mount(Editor, { props: { config: { source: 'team-prs' } } })
    expect(wrapper.get<HTMLInputElement>('[data-testid="github-source-editor-source"]').element.value).toBe('team-prs')
  })

  it('emits an immutable update:config on edit, without mutating the config prop', async () => {
    const config = { source: '' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="github-source-editor-source"]').element
    input.value = 'inbox'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.source).toBe('') // prop untouched
    expect(wrapper.emitted('update:config')).toEqual([[{ source: 'inbox' }]])
  })
})

describe('github-source validate', () => {
  it('requires a non-blank source', () => {
    expect(validate(defaults)).toEqual(['source is required'])
    expect(validate({ source: '  ' })).toEqual(['source is required'])
    expect(validate({ source: 'team-prs' })).toEqual([])
  })
})
