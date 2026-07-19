import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'

describe('feed editor', () => {
  it('renders the current feed ref', () => {
    const wrapper = mount(Editor, { props: { config: { feed: 'team-review' } } })
    expect(wrapper.get<HTMLInputElement>('[data-testid="feed-node-editor-feed"]').element.value).toBe('team-review')
  })

  it('emits an immutable update:config on edit, without mutating the config prop', async () => {
    const config = { feed: '' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="feed-node-editor-feed"]').element
    input.value = 'team-review'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.feed).toBe('')
    expect(wrapper.emitted('update:config')).toEqual([[{ feed: 'team-review' }]])
  })
})

describe('feed validate', () => {
  it('requires a non-blank feed', () => {
    expect(validate(defaults)).toEqual(['feed is required'])
    expect(validate({ feed: 'team-review' })).toEqual([])
  })
})
