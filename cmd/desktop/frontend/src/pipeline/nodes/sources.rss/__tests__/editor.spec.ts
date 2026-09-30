import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { DEFAULT_LIMIT, MAX_LIMIT, defaults, validate, type Config } from '../config'

function config(overrides: Partial<Config> = {}): Config {
  return { url: 'https://example.com/feed.xml', interval: '30m', ...overrides }
}

function typeInto(wrapper: ReturnType<typeof mount>, testid: string, value: string) {
  const input = wrapper.get<HTMLInputElement>(`[data-testid="${testid}"]`).element
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('sources.rss editor', () => {
  it('renders the current url and interval', () => {
    const wrapper = mount(Editor, { props: { config: config() } })

    expect(wrapper.get<HTMLInputElement>('[data-testid="sources.rss-editor-url"]').element.value).toBe('https://example.com/feed.xml')
    expect(wrapper.get<HTMLInputElement>('[data-testid="sources.rss-editor-interval"]').element.value).toBe('30m')
  })

  it('shows the default limit as the placeholder rather than a filled-in number', () => {
    const wrapper = mount(Editor, { props: { config: config() } })

    const limit = wrapper.get<HTMLInputElement>('[data-testid="sources.rss-editor-limit"]').element
    expect(limit.value).toBe('0')
    expect(limit.placeholder).toBe(String(DEFAULT_LIMIT))
  })

  it('emits an immutable update:config on edit, without mutating the config prop', async () => {
    const props = { config: config() }
    const wrapper = mount(Editor, { props })

    typeInto(wrapper, 'sources.rss-editor-url', 'https://other.example/atom')
    await wrapper.vm.$nextTick()

    expect(props.config.url).toBe('https://example.com/feed.xml')
    expect(wrapper.emitted('update:config')).toEqual([[{ url: 'https://other.example/atom', interval: '30m' }]])
  })

  // Zero is what an emptied number input reads as, and it means "the default",
  // which the config expresses by omitting the key.
  it('drops an emptied limit rather than writing zero', async () => {
    const wrapper = mount(Editor, { props: { config: config({ limit: 20 }) } })

    typeInto(wrapper, 'sources.rss-editor-limit', '')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ url: 'https://example.com/feed.xml', interval: '30m', limit: undefined }]])
  })

  it('drops an emptied interval rather than writing an empty string', async () => {
    const wrapper = mount(Editor, { props: { config: config() } })

    typeInto(wrapper, 'sources.rss-editor-interval', '')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ url: 'https://example.com/feed.xml', interval: undefined }]])
  })
})

describe('sources.rss validate', () => {
  it('reports the unset required field', () => {
    expect(validate(defaults)).toEqual(['a feed URL is required'])
  })

  // The three shapes a pasted feed URL takes when it is wrong.
  it('rejects a url a fetch could not use', () => {
    expect(validate(config({ url: 'example.com/feed.xml' }))).toEqual(['the feed URL must start with http:// or https://'])
    expect(validate(config({ url: 'file:///etc/feed.xml' }))).toEqual(['the feed URL must start with http:// or https://'])
    expect(validate(config({ url: 'https://user:pass@example.com/feed' }))).toEqual(['the feed URL must not contain a username or password'])
  })

  it('rejects a limit outside the range Go enforces', () => {
    const message = `limit must be a whole number from 1 to ${MAX_LIMIT}`
    expect(validate(config({ limit: 0 }))).toEqual([message])
    expect(validate(config({ limit: MAX_LIMIT + 1 }))).toEqual([message])
    expect(validate(config({ limit: 1.5 }))).toEqual([message])
  })

  // Go reads a bare number as nanoseconds and rejects it; the editor must not
  // let one through as if it meant minutes.
  it('rejects a bare number for the interval', () => {
    expect(validate(config({ interval: '30' }))).toEqual(['interval must be a duration like "1h", not a bare number'])
  })

  it('rejects a mark reference that is not a content hash', () => {
    expect(validate(config({ image: 'nope' }))).toEqual(['image is not a valid mark reference'])
  })

  it('accepts a complete config', () => {
    expect(validate(config({ limit: 25, icon: 'rss', image: 'b'.repeat(32) }))).toEqual([])
  })
})
