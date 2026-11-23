import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h, ref, type Component } from 'vue'
import FenceTree, { type FenceTreeItem } from '../FenceTree.vue'

interface Item extends FenceTreeItem {
  label: string
  children?: Item[]
}

const items: Item[] = [
  {
    id: 'root',
    label: 'root',
    children: [
      { id: 'a', label: 'a', children: [{ id: 'a1', label: 'a1' }] },
      { id: 'b', label: 'b' },
    ],
  },
]

function mountTree(selectable = true) {
  const Host = defineComponent({
    setup() {
      const selected = ref<string | null>(null)
      return () =>
        h(
          FenceTree as Component,
          {
            items,
            selectable,
            label: 't',
            testid: 't',
            selected: selected.value,
            'onUpdate:selected': (id: string | null) => (selected.value = id),
          },
          { row: ({ item }: { item: Item }) => item.label },
        )
    },
  })
  return mount(Host, { attachTo: document.body })
}

const visible = (wrapper: ReturnType<typeof mountTree>) => wrapper.findAll('[role="treeitem"]').map((row) => row.text())

describe('FenceTree', () => {
  it('renders treeitems with levels and expanded state', () => {
    const wrapper = mountTree()
    expect(wrapper.find('[role="tree"]').exists()).toBe(true)
    expect(visible(wrapper)).toEqual(['root', 'a', 'a1', 'b'])
    expect(wrapper.get('[data-testid="t-row-a1"]').attributes('aria-level')).toBe('3')
    expect(wrapper.get('[data-testid="t-row-a"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[data-testid="t-row-b"]').attributes('aria-expanded')).toBeUndefined()
  })

  it('folds a row from its twisty', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-twisty-a"]').trigger('click')
    expect(visible(wrapper)).toEqual(['root', 'a', 'b'])
    expect(wrapper.get('[data-testid="t-row-a"]').attributes('aria-expanded')).toBe('false')
  })

  it('moves the selection with ArrowDown and ArrowUp over visible rows', async () => {
    const wrapper = mountTree()
    const tree = wrapper.get('[role="tree"]')
    await tree.trigger('keydown', { key: 'ArrowDown' })
    expect(tree.attributes('aria-activedescendant')).toContain('root')
    await tree.trigger('keydown', { key: 'ArrowDown' })
    await tree.trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.get('[data-testid="t-row-a1"]').attributes('aria-selected')).toBe('true')
    await tree.trigger('keydown', { key: 'ArrowUp' })
    expect(wrapper.get('[data-testid="t-row-a"]').attributes('aria-selected')).toBe('true')
  })

  it('selects on click and emits activate on click and Enter', async () => {
    const wrapper = mountTree()
    const tree = wrapper.getComponent(FenceTree)
    await wrapper.get('[data-testid="t-row-b"]').trigger('click')
    expect(wrapper.get('[data-testid="t-row-b"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[role="tree"]').trigger('keydown', { key: 'Enter' })
    expect(tree.emitted('activate')).toEqual([['b'], ['b']])
  })

  it('moves the selection to the folded row when a selected descendant is hidden', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-a1"]').trigger('click')
    await wrapper.get('[data-testid="t-twisty-a"]').trigger('click')
    expect(wrapper.get('[data-testid="t-row-a"]').attributes('aria-selected')).toBe('true')
  })

  it('folds with ArrowLeft, selects the parent on a second ArrowLeft, and unfolds with ArrowRight', async () => {
    const wrapper = mountTree()
    const tree = wrapper.get('[role="tree"]')
    await wrapper.get('[data-testid="t-row-a"]').trigger('click')
    await tree.trigger('keydown', { key: 'ArrowLeft' })
    expect(visible(wrapper)).toEqual(['root', 'a', 'b'])
    await tree.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.get('[data-testid="t-row-root"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[data-testid="t-row-a"]').trigger('click')
    await tree.trigger('keydown', { key: 'ArrowRight' })
    expect(visible(wrapper)).toEqual(['root', 'a', 'a1', 'b'])
  })

  it('is not focusable or selectable without selectable', async () => {
    const wrapper = mountTree(false)
    expect(wrapper.get('[role="tree"]').attributes('tabindex')).toBeUndefined()
    await wrapper.get('[data-testid="t-row-b"]').trigger('click')
    expect(wrapper.get('[data-testid="t-row-b"]').attributes('aria-selected')).toBeUndefined()
  })

  it('collapses and expands every row through its exposed methods', async () => {
    const wrapper = mountTree()
    const tree = wrapper.getComponent(FenceTree)
    await wrapper.get('[data-testid="t-row-a1"]').trigger('click')
    ;(tree.vm as unknown as { collapseAll(): void }).collapseAll()
    await wrapper.vm.$nextTick()
    expect(visible(wrapper)).toEqual(['root'])
    expect(wrapper.get('[data-testid="t-row-root"]').attributes('aria-selected')).toBe('true')
    ;(tree.vm as unknown as { expandAll(): void }).expandAll()
    await wrapper.vm.$nextTick()
    expect(visible(wrapper)).toEqual(['root', 'a', 'a1', 'b'])
  })
})
