import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DriveTree from '../DriveTree.vue'
import { doc, folder, root } from '../../../test-utils/drive'

const events = vi.hoisted(() => ({ On: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Events: { On: events.On } }))

const tree = root([
  folder('canvases', [doc('canvases/report.canvas.json', { title: 'E2E Report' })]),
  folder('plans', [
    folder('plans/old', [doc('plans/old/a.md')]),
    doc('plans/2026-10-11-plan.md', { title: 'The Plan' }),
  ]),
  doc('index.html'),
  doc('notes.txt'),
])

function mountTree(props: Partial<{ selected: string | null; filter: string }> = {}) {
  return mount(DriveTree, {
    props: { space: 'acme/site', tree, selected: null, filter: '', testid: 't', ...props },
    attachTo: document.body,
  })
}

const rowPaths = (wrapper: ReturnType<typeof mountTree>) =>
  wrapper.findAll('[role="treeitem"]').map((row) => row.attributes('data-testid')?.replace('t-row-', ''))

describe('DriveTree', () => {
  beforeEach(() => {
    localStorage.clear()
    events.On.mockReturnValue(() => {})
  })

  it('lists folders first as served, folded, with a kind per row', () => {
    const wrapper = mountTree()
    expect(rowPaths(wrapper)).toEqual(['canvases', 'plans', 'index.html', 'notes.txt'])
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('[data-testid="t-row-index.html"]').attributes('data-kind')).toBe('html')
    expect(wrapper.get('[data-testid="t-row-notes.txt"]').attributes('data-kind')).toBe('file')
  })

  it('opens and closes a folder on click, and emits pick for a document', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-plans"]').trigger('click')
    expect(rowPaths(wrapper)).toEqual([
      'canvases',
      'plans',
      'plans/old',
      'plans/2026-10-11-plan.md',
      'index.html',
      'notes.txt',
    ])
    expect(wrapper.get('[data-testid="t-row-plans/2026-10-11-plan.md"]').text()).toBe('The Plan')

    await wrapper.get('[data-testid="t-row-plans/2026-10-11-plan.md"]').trigger('click')
    expect(wrapper.emitted('pick')).toEqual([['plans/2026-10-11-plan.md']])

    await wrapper.get('[data-testid="t-row-plans"]').trigger('click')
    expect(rowPaths(wrapper)).toEqual(['canvases', 'plans', 'index.html', 'notes.txt'])
  })

  it('opens every folder while filtering and keeps only the matches and their folders', async () => {
    const wrapper = mountTree({ filter: 'a.md' })
    expect(rowPaths(wrapper)).toEqual(['plans', 'plans/old', 'plans/old/a.md'])

    await wrapper.setProps({ filter: 'report' })
    expect(rowPaths(wrapper)).toEqual(['canvases', 'canvases/report.canvas.json'])
  })

  it('walks the rows with the keyboard, opening a folder and picking a document', async () => {
    const wrapper = mountTree()
    const host = wrapper.get('[data-testid="t-tree"]')
    await host.trigger('keydown', { key: 'ArrowDown' })
    await host.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('true')

    await host.trigger('keydown', { key: 'ArrowDown' })
    await host.trigger('keydown', { key: 'ArrowDown' })
    await host.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('pick')).toEqual([['plans/2026-10-11-plan.md']])

    await host.trigger('keydown', { key: 'ArrowLeft' })
    await host.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')
  })

  it('reveals the selected document by opening its folders and scrolling to it', async () => {
    const scrolled: string[] = []
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolled.push(this.getAttribute('data-testid') ?? '')
    }
    try {
      const wrapper = mountTree({ selected: 'plans/old/a.md' })
      await flushPromises()
      expect(wrapper.get('[data-testid="t-row-plans/old/a.md"]').attributes('aria-selected')).toBe('true')
      expect(wrapper.get('[data-testid="t-row-plans/old"]').attributes('aria-expanded')).toBe('true')
      expect(scrolled).toContain('t-row-plans/old/a.md')
    } finally {
      Element.prototype.scrollIntoView = original
    }
  })

  it('emits menu with the row as its anchor on a right click', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-index.html"]').trigger('contextmenu')
    const [entry, anchor] = wrapper.emitted('menu')?.[0] ?? []
    expect((entry as { path: string }).path).toBe('index.html')
    expect((anchor as HTMLElement).getAttribute('data-testid')).toBe('t-row-index.html')
  })
})
