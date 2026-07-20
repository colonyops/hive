import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate, type Config } from '../config'

function fire(el: Element, type: string) {
  el.dispatchEvent(new Event(type, { bubbles: true }))
}

describe('github-filter editor', () => {
  it('renders populated glob groups and checked toggles', () => {
    const config: Config = { repos: ['acme/*'], types: ['pr'], reasons: ['mention'] }
    const wrapper = mount(Editor, { props: { config } })

    expect(wrapper.get<HTMLTextAreaElement>('[data-testid="github-filter-editor-repos"]').element.value).toBe('acme/*')
    expect(wrapper.get<HTMLInputElement>('[data-testid="github-filter-editor-type-pr"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-testid="github-filter-editor-type-issue"]').element.checked).toBe(false)
    expect(wrapper.get<HTMLInputElement>('[data-testid="github-filter-editor-reason-mention"]').element.checked).toBe(true)
  })

  it('emits an immutable update:config from a glob group edit', async () => {
    const config: Config = {}
    const wrapper = mount(Editor, { props: { config } })

    const textarea = wrapper.get<HTMLTextAreaElement>('[data-testid="github-filter-editor-exclude-authors"]').element
    textarea.value = '*[bot]\nghost'
    fire(textarea, 'input')
    await wrapper.vm.$nextTick()

    expect(config).toEqual({}) // prop untouched
    expect(wrapper.emitted('update:config')).toEqual([[{ exclude_authors: ['*[bot]', 'ghost'] }]])
  })

  it('emits an immutable update:config from a type toggle', async () => {
    const config: Config = {}
    const wrapper = mount(Editor, { props: { config } })

    await wrapper.get('[data-testid="github-filter-editor-type-issue"]').setValue(true)

    expect(config).toEqual({})
    expect(wrapper.emitted('update:config')).toEqual([[{ types: ['issue'] }]])
  })

  it('emits an immutable update:config from a reason toggle, clearing the key once empty again', async () => {
    const config: Config = { reasons: ['mention'] }
    const wrapper = mount(Editor, { props: { config } })

    const checkbox = wrapper.get<HTMLInputElement>('[data-testid="github-filter-editor-reason-mention"]').element
    checkbox.checked = false
    fire(checkbox, 'change')
    await wrapper.vm.$nextTick()

    expect(config).toEqual({ reasons: ['mention'] })
    expect(wrapper.emitted('update:config')).toEqual([[{ reasons: undefined }]])
  })
})

describe('github-filter validate', () => {
  it('flags an entirely empty filter', () => {
    expect(validate(defaults)).toEqual(['at least one filter group must be set'])
  })

  it('passes once any group is non-empty', () => {
    expect(validate({ repos: ['acme/*'] })).toEqual([])
    expect(validate({ reasons: ['mention'] })).toEqual([])
  })
})
