import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import type { CanvasMarkdownPart } from '../../../lib/githubMarkdown'
import { fakePlatform } from '../../../test-utils/platform'
import CanvasMarkdown from '../../CanvasMarkdown.vue'
import FenceHost from '../FenceHost.vue'
import { isFence, registerFence, type FenceProps } from '../registry'

const ctx = { docPath: 'a.md', paths: new Set<string>(), follow: () => {} }
const part = (lang: string, source: string): Extract<CanvasMarkdownPart, { kind: 'fence' }> => ({
  kind: 'fence',
  lang,
  source,
  lines: [3, 5],
  codeStart: 4,
})

const cleanup: (() => void)[] = []
afterEach(() => {
  while (cleanup.length) cleanup.pop()!()
})

function register(lang: string, component: ReturnType<typeof defineComponent>, extra: object = {}) {
  cleanup.push(registerFence({ lang, builtin: false, body: 'yaml', component, ...extra }))
}

const Echo = defineComponent({
  props: ['data', 'source', 'ctx', 'testid'],
  setup: (props: FenceProps) => () => h('div', { class: 'echo' }, JSON.stringify(props.data)),
})

describe('FenceHost', () => {
  it('falls back to the highlighted code block when no renderer is registered', () => {
    const wrapper = mount(FenceHost, { props: { part: part('nope', 'a: 1\n'), ctx, testid: 'd-fence-0' } })
    const root = wrapper.get('.canvas-fence')
    expect(root.attributes()).toMatchObject({
      'data-fence-lang': 'nope',
      'data-source-start': '3',
      'data-source-end': '5',
      'data-testid': 'd-fence-0',
    })
    expect(root.get('.markdown-code').attributes('data-source-code-start')).toBe('4')
    expect(root.text()).toContain('a: 1')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('parses the body once and hands the renderer data, source, ctx and testid', () => {
    register('t-echo', Echo)
    const wrapper = mount(FenceHost, { props: { part: part('t-echo', 'a: [1, 2]\n'), ctx, testid: 'd-fence-1' } })
    expect(wrapper.get('.echo').text()).toBe('{"a":[1,2]}')
    const props = wrapper.getComponent(Echo).props()
    expect(props).toMatchObject({ source: 'a: [1, 2]\n', testid: 'd-fence-1', ctx })
  })

  it('hands a text-bodied renderer the raw source', () => {
    register('t-text', Echo, { body: 'text' })
    const wrapper = mount(FenceHost, { props: { part: part('t-text', 'a: [\n'), ctx, testid: 't' } })
    expect(wrapper.get('.echo').text()).toBe('"a: [\\n"')
  })

  it('runs validate and passes the narrowed data', () => {
    register('t-valid', Echo, { validate: () => ({ ok: true, data: { narrowed: true } }) })
    const wrapper = mount(FenceHost, { props: { part: part('t-valid', 'a: 1\n'), ctx, testid: 't' } })
    expect(wrapper.get('.echo').text()).toBe('{"narrowed":true}')
  })

  it('shows a YAML error above the raw source', () => {
    register('t-yaml', Echo)
    const wrapper = mount(FenceHost, { props: { part: part('t-yaml', 'a: [1, 2\nb: 3\n'), ctx, testid: 'd-fence-2' } })
    expect(wrapper.get('[data-testid="d-fence-2-error"]').text()).toMatch(/^Invalid YAML/)
    expect(wrapper.find('.echo').exists()).toBe(false)
    expect(wrapper.get('.markdown-code').text()).toContain('a: [1, 2')
  })

  it('shows a validation error above the raw source', () => {
    register('t-bad', Echo, { validate: () => ({ ok: false, error: '"files" must be a list' }) })
    const wrapper = mount(FenceHost, { props: { part: part('t-bad', 'x: 1\n'), ctx, testid: 'd-fence-3' } })
    expect(wrapper.get('[data-testid="d-fence-3-error"]').text()).toBe('"files" must be a list')
    expect(wrapper.get('.markdown-code').text()).toContain('x: 1')
  })

  it('falls back to the code block, and logs, when the renderer throws', async () => {
    const Boom = defineComponent({
      setup: () => () => {
        throw new Error('boom')
      },
    })
    register('t-boom', Boom)
    const wrapper = mount(FenceHost, { props: { part: part('t-boom', 'a: 1\n'), ctx, testid: 't' } })
    await nextTick()
    expect(wrapper.find('.markdown-code').exists()).toBe(true)
    expect(fakePlatform.logger.warn).toHaveBeenCalledWith(
      'fence renderer failed',
      expect.objectContaining({ lang: 't-boom' }),
    )
  })
})

describe('CanvasMarkdown fences', () => {
  it('re-splits a mounted document when a fence registers and unregisters', async () => {
    const wrapper = mount(CanvasMarkdown, {
      props: { source: 'Hi\n\n```t-late\na: 1\n```\n', testid: 'doc' },
    })
    expect(wrapper.find('[data-testid="doc-fence-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="doc-html-0"]').text()).toContain('a: 1')

    const unregister = registerFence({ lang: 't-late', builtin: false, body: 'yaml', component: Echo })
    await flushPromises()
    expect(wrapper.get('[data-testid="doc-fence-1"] .echo').text()).toBe('{"a":1}')

    unregister()
    await flushPromises()
    expect(wrapper.find('[data-testid="doc-fence-1"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="doc-html-0"]').text()).toContain('a: 1')
  })

  it('gives a fence a context that follows through the follow event', async () => {
    const Link = defineComponent({
      props: ['ctx'],
      setup: (props: Pick<FenceProps, 'ctx'>) => () =>
        h('button', { onClick: () => props.ctx.follow('plans/x.md') }, 'go'),
    })
    cleanup.push(registerFence({ lang: 't-follow', builtin: false, body: 'yaml', component: Link }))
    const paths = new Set(['plans/x.md'])
    const wrapper = mount(CanvasMarkdown, {
      props: { source: '```t-follow\na: 1\n```', testid: 'doc', links: { docPath: 'a.md', paths } },
    })
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('follow')).toEqual([['plans/x.md']])
  })

  it('renders [[wikilinks]] only with links', () => {
    const paths = new Set(['plans/x.md'])
    const on = mount(CanvasMarkdown, { props: { source: '[[x]]', testid: 'd', links: { docPath: 'a.md', paths } } })
    expect(on.get('a.wikilink').attributes('href')).toBe('.hive/plans/x.md')
    const off = mount(CanvasMarkdown, { props: { source: '[[x]]', testid: 'd' } })
    expect(off.text()).toBe('[[x]]')
  })
})

describe('registerFence', () => {
  it('rejects a duplicate lang and a reserved user lang, and logs a warning', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    register('t-dup', Echo)
    registerFence({ lang: 't-dup', builtin: false, body: 'yaml', component: Echo })()
    registerFence({ lang: 'hive-nope', builtin: false, body: 'yaml', component: Echo })()
    expect(warn).toHaveBeenCalledTimes(2)
    expect(isFence('hive-nope')).toBe(false)
    expect(isFence('t-dup')).toBe(true)
    warn.mockRestore()
  })

  it('a rejected registration does not unregister the original renderer', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    register('t-keep', Echo)
    registerFence({ lang: 't-keep', builtin: false, body: 'yaml', component: Echo })()
    expect(isFence('t-keep')).toBe(true)
  })

  it('allows a builtin hive- lang and unregisters it', () => {
    const off = registerFence({ lang: 'hive-t-builtin', builtin: true, body: 'yaml', component: Echo })
    const mounted = () =>
      mount(FenceHost, { props: { part: part('hive-t-builtin', 'a: 1\n'), ctx, testid: 't' } })
        .find('.echo')
        .exists()
    expect(mounted()).toBe(true)
    off()
    expect(mounted()).toBe(false)
  })
})
