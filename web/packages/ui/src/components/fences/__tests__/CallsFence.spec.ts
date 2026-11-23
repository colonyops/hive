import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { CallData } from '../../../lib/fenceData'
import { fakePlatform } from '../../../test-utils/platform'
import CallsFence from '../CallsFence.vue'

const data: CallData = {
  name: 'parts',
  at: 'a/Canvas.vue:22',
  calls: [
    {
      name: 'render',
      at: 'a/md.ts:280',
      note: 'splits',
      calls: [
        { name: 'isFence', at: 'a/reg.ts:31', focus: true, explain: 'The decision point.' },
        { name: 'parse', at: 'markdown-it' },
      ],
    },
    { name: 'Host', at: 'a/Host.vue:9', more: 2 },
  ],
}

function mountCalls(value: CallData = data) {
  return mount(CallsFence, {
    props: {
      data: value,
      source: 'name: parts',
      ctx: { docPath: 'x.md', paths: new Set<string>(), follow: () => {} },
      testid: 'c',
    },
    attachTo: document.body,
  })
}

const strip = (wrapper: ReturnType<typeof mountCalls>, part: string) => wrapper.get(`[data-testid="c-strip-${part}"]`)

describe('CallsFence', () => {
  it('renders the tree with kinds, notes, location chips and a more row', () => {
    const wrapper = mountCalls()
    expect(wrapper.text()).toContain('5 calls')
    expect(wrapper.text()).toContain('splits')
    expect(wrapper.text()).toContain('md.ts:280')
    expect(wrapper.text()).toContain('2 more')
    expect(wrapper.get('[data-testid="c-tree-row-0.0.1"] .calls-kind').classes()).toContain('external')
    expect(wrapper.get('[data-testid="c-tree-row-0.1"] .calls-kind').classes()).toContain('component')
  })

  it('colors chips by file so the same file matches and others differ', () => {
    const wrapper = mountCalls({
      name: 'r',
      at: 'a.ts:1',
      calls: [
        { name: 'x', at: 'a.ts:9' },
        { name: 'y', at: 'b.ts:2' },
      ],
    })
    const color = (id: string) => wrapper.get(`[data-testid="c-loc-${id}"]`).attributes('style')
    expect(color('0')).toBe(color('0.0'))
    expect(color('0')).not.toBe(color('0.1'))
  })

  it('starts on the focus call and fills the strip', () => {
    const wrapper = mountCalls()
    expect(wrapper.get('[data-testid="c-tree-row-0.0.0"]').attributes('aria-selected')).toBe('true')
    expect(strip(wrapper, 'name').text()).toBe('isFence')
    expect(strip(wrapper, 'explain').text()).toBe('The decision point.')
    expect(strip(wrapper, 'path').text()).toContain('parts')
    expect(strip(wrapper, 'path').text()).toContain('render')
    expect(wrapper.get('[data-testid="c-caller-0.0"]').text()).toBe('render')
  })

  it('starts on the root without a focus call, and the explain defaults to the note', () => {
    const wrapper = mountCalls({ name: 'r', calls: [{ name: 'k', note: 'one line' }] })
    expect(strip(wrapper, 'name').text()).toBe('r')
    expect(wrapper.get('[data-testid="c-callee-0.0"]').text()).toBe('k')
  })

  it('selects from a click, the relation chips and the path', async () => {
    const wrapper = mountCalls()
    await wrapper.get('[data-testid="c-tree-row-0.1"]').trigger('click')
    expect(strip(wrapper, 'name').text()).toBe('Host')
    await wrapper.get('[data-testid="c-caller-0"]').trigger('click')
    expect(strip(wrapper, 'name').text()).toBe('parts')
    await wrapper.get('[data-testid="c-callee-0.0"]').trigger('click')
    expect(strip(wrapper, 'name').text()).toBe('render')
    await wrapper.get('[data-testid="c-callee-0.0.1"]').trigger('click')
  })

  it('steps with the arrow keys', async () => {
    const wrapper = mountCalls()
    const tree = wrapper.get('[role="tree"]')
    await tree.trigger('keydown', { key: 'ArrowDown' })
    expect(strip(wrapper, 'name').text()).toBe('parse')
    await tree.trigger('keydown', { key: 'ArrowUp' })
    await tree.trigger('keydown', { key: 'ArrowUp' })
    expect(strip(wrapper, 'name').text()).toBe('render')
  })

  it('explains an elided row without a call', async () => {
    const wrapper = mountCalls()
    await wrapper.get('[data-testid="c-tree-row-0.1.more"]').trigger('click')
    expect(strip(wrapper, 'explain').text()).toBe('2 more calls not shown.')
  })

  it('copies the full location from a chip without selecting the row', async () => {
    const wrapper = mountCalls()
    await wrapper.get('[data-testid="c-loc-0.1"]').trigger('click')
    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('a/Host.vue:9')
    expect(wrapper.get('[data-testid="c-tree-row-0.0.0"]').attributes('aria-selected')).toBe('true')
  })
})
