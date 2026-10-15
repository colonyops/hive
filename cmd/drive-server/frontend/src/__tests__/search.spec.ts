import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import SearchSnippet from '../components/SearchSnippet.vue'
import ToolbarSearch from '../components/ToolbarSearch.vue'
import { ApiError } from '../api'
import { snippetParts } from '../lib/snippet'
import { spaceFromKey } from '../lib/space'
import { doc } from './fixtures'

const search = vi.hoisted(() => vi.fn())
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api: { search },
}))

const start = ''
const end = ''

describe('snippetParts', () => {
  it('splits matched runs from plain text', () => {
    expect(snippetParts(`a ${start}b${end} c`)).toEqual([
      { text: 'a ', match: false },
      { text: 'b', match: true },
      { text: ' c', match: false },
    ])
  })

  it('returns plain text when nothing is marked', () => {
    expect(snippetParts('plain')).toEqual([{ text: 'plain', match: false }])
  })
})

describe('SearchSnippet', () => {
  it('renders server text as text and marks only the matched runs', () => {
    const wrapper = mount(SearchSnippet, { props: { snippet: `<img src=x onerror=1> ${start}<b>hit</b>${end}` } })

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('b').exists()).toBe(false)
    expect(wrapper.text()).toBe('<img src=x onerror=1> <b>hit</b>')
    const marks = wrapper.findAll('mark')
    expect(marks).toHaveLength(1)
    expect(marks[0].text()).toBe('<b>hit</b>')
  })
})

function mountSearch(spaceKey: string | null = 'acme/notes') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:p(.*)*', component: {} }] })
  const wrapper = mount(ToolbarSearch, {
    props: {
      space: spaceKey ? spaceFromKey(spaceKey) : null,
      modelValue: '',
      'onUpdate:modelValue': (value: string) => wrapper.setProps({ modelValue: value }),
    },
    attachTo: document.body,
    global: { plugins: [router] },
  })
  return { router, wrapper }
}

const input = () => document.body.querySelector<HTMLInputElement>('[data-testid="search-input"]')!

describe('ToolbarSearch keyboard', () => {
  it('focuses the field on "/" and on Ctrl+K', () => {
    mountSearch()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: '/', cancelable: true }))
    expect(document.activeElement).toBe(input())

    input().blur()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, cancelable: true }))
    expect(document.activeElement).toBe(input())
  })

  it('leaves "/" alone while the user types in another field', () => {
    mountSearch()
    const other = document.createElement('input')
    document.body.append(other)
    other.focus()

    other.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true }))

    expect(document.activeElement).toBe(other)
    other.remove()
  })
})

describe('ToolbarSearch results', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    search.mockReset()
  })
  afterEach(() => vi.useRealTimers())

  async function type(wrapper: ReturnType<typeof mountSearch>['wrapper'], text: string) {
    input().focus()
    input().value = text
    input().dispatchEvent(new Event('input'))
    await vi.advanceTimersByTimeAsync(250)
    await flushPromises()
    void wrapper
  }

  it('debounces, scopes to the current space, and shows title, path, and marked snippet', async () => {
    search.mockResolvedValue({
      hits: [{ document: doc({ path: 'docs/setup.md', title: 'Setup' }), snippet: `how to ${start}install${end}` }],
    })
    const { wrapper } = mountSearch()

    await type(wrapper, 'inst')

    expect(search).toHaveBeenCalledTimes(1)
    expect(search).toHaveBeenCalledWith('inst', 'acme/notes')
    const hit = document.body.querySelector('[data-testid="search-hit"]')!
    expect(hit.textContent).toContain('Setup')
    expect(hit.textContent).toContain('acme/notes / docs/setup.md')
    expect(hit.querySelector('mark')?.textContent).toBe('install')
  })

  it('searches every space after the toggle', async () => {
    search.mockResolvedValue({ hits: [] })
    const { wrapper } = mountSearch()
    await type(wrapper, 'inst')

    document.body.querySelector<HTMLElement>('[data-testid="search-scope-all"]')!.click()
    await vi.advanceTimersByTimeAsync(250)
    await flushPromises()

    expect(search).toHaveBeenLastCalledWith('inst', undefined)
  })

  it('opens the highlighted hit with Enter', async () => {
    search.mockResolvedValue({ hits: [{ document: doc({ path: 'a b.md' }), snippet: '' }] })
    const { wrapper, router } = mountSearch()
    await type(wrapper, 'a')

    input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/repos/acme/notes/a%20b.md')
  })

  it('stays usable when the server has no search endpoint', async () => {
    search.mockRejectedValue(new ApiError(404, 'not_found', 'no such endpoint'))
    const { wrapper } = mountSearch()

    await type(wrapper, 'inst')

    expect(document.body.querySelector('[data-testid="search-unavailable"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="search-error"]')).toBeNull()
  })
})
