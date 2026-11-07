import { computed, ref, type ComputedRef, type Ref, type StyleValue } from 'vue'
import { useMutationObserver, useVirtualList } from '@vueuse/core'

/** A tree row's height in rem. The rows' CSS must match it: every offset is computed from it, never measured. */
export const TREE_ROW_REM = 1.625

function rootFontPx(): number {
  return parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
}

const FIRST_FRAME_ROWS = 50

export interface VirtualRow<T> {
  data: T
  index: number
  /** The row's 1-based place among its siblings, and their count, for aria-posinset and aria-setsize. */
  posinset: number
  setsize: number
  style: StyleValue
}

/**
 * Sibling positions for a flattened tree, where a child's depth is its parent's plus one.
 * A virtualized tree needs them on every row, since assistive tech cannot count rows that are not drawn.
 */
export function treePositions(depths: readonly number[]): { posinset: number; setsize: number }[] {
  const lastAt: number[] = []
  const counts = new Map<string, number>()
  const groups: string[] = []
  const positions = depths.map((depth, index) => {
    const group = `${depth}:${depth === 0 ? -1 : (lastAt[depth - 1] ?? -1)}`
    const posinset = (counts.get(group) ?? 0) + 1
    counts.set(group, posinset)
    groups.push(group)
    lastAt[depth] = index
    lastAt.length = depth + 1
    return { posinset, setsize: 0 }
  })
  positions.forEach((position, index) => (position.setsize = counts.get(groups[index]) ?? 0))
  return positions
}

/**
 * Draws only the rows near a tree's viewport. Rows are absolutely placed at
 * index × the row height, so `pinned` rows can stay drawn out of range — a rename
 * field, a drag source, or a menu anchor that must not unmount mid-use.
 *
 * useVirtualList supplies the range. Its own scrollTo is not used: its
 * `nearest` always moves the row to an edge, which jolts a keyboard walk.
 */
export function useVirtualTree<T extends { depth: number }>(
  rows: Ref<T[]> | ComputedRef<T[]>,
  pinned: () => readonly number[] = () => [],
) {
  // The interface scale sets the root font size through a style on <html>, so
  // a row's px height follows that style.
  const rowPx = ref(TREE_ROW_REM * rootFontPx())
  useMutationObserver(document.documentElement, () => (rowPx.value = TREE_ROW_REM * rootFontPx()), {
    attributes: true,
    attributeFilter: ['style'],
  })
  const { list, containerProps } = useVirtualList(rows, { itemHeight: () => rowPx.value, overscan: 10 })
  const positions = computed(() => treePositions(rows.value.map((row) => row.depth)))

  const rendered = computed<VirtualRow<T>[]>(() => {
    // useVirtualList fills its range from a watcher after mount; until then
    // the head of the list stands in, so the first frame is not blank.
    const range = list.value.length ? list.value.map((item) => item.index) : [...Array(FIRST_FRAME_ROWS).keys()]
    const indexes = new Set(range)
    for (const index of pinned()) if (index >= 0 && index < rows.value.length) indexes.add(index)
    return [...indexes]
      .filter((index) => index < rows.value.length)
      .sort((a, b) => a - b)
      .map((index) => ({
        data: rows.value[index],
        index,
        ...positions.value[index],
        style: { position: 'absolute', top: `${index * rowPx.value}px`, left: 0, right: 0 },
      }))
  })

  const spacerStyle = computed<StyleValue>(() => ({
    position: 'relative',
    height: `${rows.value.length * rowPx.value}px`,
  }))

  /** Scrolls the least distance that shows the row whole. */
  function reveal(index: number): void {
    const scroller = containerProps.ref.value
    if (!scroller || index < 0) return
    const top = index * rowPx.value
    if (top < scroller.scrollTop) scroller.scrollTop = top
    else if (top + rowPx.value > scroller.scrollTop + scroller.clientHeight)
      scroller.scrollTop = top + rowPx.value - scroller.clientHeight
  }

  return { rendered, containerProps, spacerStyle, reveal, rowPx }
}
