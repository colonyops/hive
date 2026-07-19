import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate, type Config } from '../config'

describe('github-source editor', () => {
  it('renders the current kind and query', () => {
    const config: Config = { kind: 'search', query: 'is:open is:pr' }
    const wrapper = mount(Editor, { props: { config } })
    expect(wrapper.get<HTMLSelectElement>('[data-testid="github-source-editor-kind"]').element.value).toBe('search')
    expect(wrapper.get<HTMLInputElement>('[data-testid="github-source-editor-query"]').element.value).toBe('is:open is:pr')
  })

  it('emits an immutable update:config on query edit, without mutating the config prop', async () => {
    const config: Config = { kind: 'search', query: '' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="github-source-editor-query"]').element
    input.value = 'is:open'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.query).toBe('') // prop untouched
    expect(wrapper.emitted('update:config')).toEqual([[{ kind: 'search', query: 'is:open' }]])
  })

  it('hides the query field and clears it when switching to notifications', async () => {
    const config: Config = { kind: 'search', query: 'is:open' }
    const wrapper = mount(Editor, { props: { config } })

    const select = wrapper.get<HTMLSelectElement>('[data-testid="github-source-editor-kind"]').element
    select.value = 'notifications'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ kind: 'notifications', query: '' }]])
  })
})

describe('github-source validate', () => {
  it('requires a query for search sources', () => {
    expect(validate(defaults)).toEqual(['a search source requires a query'])
    expect(validate({ kind: 'search', query: '  ' })).toEqual(['a search source requires a query'])
    expect(validate({ kind: 'search', query: 'is:open' })).toEqual([])
  })

  it('rejects a query on notifications sources', () => {
    expect(validate({ kind: 'notifications' })).toEqual([])
    expect(validate({ kind: 'notifications', query: 'is:open' })).toEqual(['a notifications source takes no query'])
  })

  it('enforces per-kind limit caps', () => {
    expect(validate({ kind: 'search', query: 'x', limit: 101 })).toEqual(['search limit caps at 100'])
    expect(validate({ kind: 'notifications', limit: 51 })).toEqual(['notifications limit caps at 50'])
  })
})
