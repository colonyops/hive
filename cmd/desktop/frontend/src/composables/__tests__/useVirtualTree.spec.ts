import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { TREE_ROW_REM, treePositions, useVirtualTree } from '../useVirtualTree'

// happy-dom's root font is 16px.
const TREE_ROW_PX = TREE_ROW_REM * 16

describe('treePositions', () => {
  it('numbers each row among its siblings', () => {
    // a, a/x, a/y, a/y/z, b
    expect(treePositions([0, 1, 1, 2, 0])).toEqual([
      { posinset: 1, setsize: 2 },
      { posinset: 1, setsize: 2 },
      { posinset: 2, setsize: 2 },
      { posinset: 1, setsize: 1 },
      { posinset: 2, setsize: 2 },
    ])
  })

  it('keeps two folders’ children in separate sets', () => {
    // a, a/x, b, b/y, b/z
    expect(treePositions([0, 1, 0, 1, 1]).map((p) => p.setsize)).toEqual([2, 1, 2, 2, 2])
  })
})

function mountList(count: number, pinned: number[] = []) {
  const rows = ref(Array.from({ length: count }, (_, index) => ({ depth: 0, index })))
  let api: ReturnType<typeof useVirtualTree<{ depth: number; index: number }>> | undefined
  const wrapper = mount(
    defineComponent({
      setup() {
        api = useVirtualTree(rows, () => pinned)
        return () =>
          h('div', { ...api!.containerProps, 'data-testid': 'scroller' }, [
            h(
              'div',
              { style: api!.spacerStyle.value },
              api!.rendered.value.map((row) => h('div', { key: row.index, 'data-row': row.index })),
            ),
          ])
      },
    }),
    { attachTo: document.body },
  )
  const scroller = wrapper.get('[data-testid="scroller"]').element as HTMLElement
  Object.defineProperty(scroller, 'clientHeight', { value: 10 * TREE_ROW_PX })
  return { wrapper, scroller, api: api! }
}

const drawn = (wrapper: ReturnType<typeof mountList>['wrapper']) =>
  wrapper.findAll('[data-row]').map((row) => Number(row.attributes('data-row')))

describe('useVirtualTree', () => {
  it('draws only the rows near the viewport of a long list, plus the pinned ones', async () => {
    const { wrapper, scroller } = mountList(1000, [900])
    scroller.scrollTop = 500 * TREE_ROW_PX
    await wrapper.get('[data-testid="scroller"]').trigger('scroll')
    const rows = drawn(wrapper)
    expect(rows.length).toBeLessThan(40)
    expect(rows).toContain(505)
    expect(rows).toContain(900)
    expect(rows).not.toContain(0)
  })

  it('scrolls the least distance that shows a row', () => {
    const { scroller, api } = mountList(100)
    api.reveal(5)
    expect(scroller.scrollTop).toBe(0)
    api.reveal(20)
    expect(scroller.scrollTop).toBe(21 * TREE_ROW_PX - 10 * TREE_ROW_PX)
    api.reveal(15)
    expect(scroller.scrollTop).toBe(21 * TREE_ROW_PX - 10 * TREE_ROW_PX)
    api.reveal(3)
    expect(scroller.scrollTop).toBe(3 * TREE_ROW_PX)
  })

  it('follows the interface scale, which sets the root font size', async () => {
    const { api } = mountList(10)
    try {
      document.documentElement.style.fontSize = '20px'
      await flushPromises()
      expect(api.rowPx.value).toBe(TREE_ROW_REM * 20)
    } finally {
      document.documentElement.style.fontSize = ''
    }
  })
})
