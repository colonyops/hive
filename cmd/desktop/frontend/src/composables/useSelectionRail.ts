import { useResizeObserver } from '@vueuse/core'
import { nextTick, onScopeDispose, ref, watch, type Ref, type WatchSource } from 'vue'

export interface SelectionRail {
  y: number
  height: number
  shown: boolean
}

const SETTLE_MS = 260

/**
 * The traveling selection marks of a tree sidebar, one per selector. Each rail
 * is measured off the first row in `content` that matches its selector, so a
 * change of selection reads as the same mark relocating. Rows can move without
 * a selection change, so a resize of `content` and every `sources` change
 * re-measure. While `settle()` holds, a measurement keeps re-running per frame
 * for a moment, for rows that move under FLIP or inside an animating panel.
 */
export function useSelectionRail(
  content: Ref<HTMLElement | null>,
  selectors: string[],
  options: { sources?: WatchSource[]; settle?: () => boolean } = {},
): { rails: Ref<SelectionRail[]>; update: () => void } {
  const rails = ref<SelectionRail[]>(selectors.map(() => ({ y: 0, height: 0, shown: false })))

  // A rail with no row to sit on fades out where it stands rather than
  // resetting, so it does not travel from a stale origin when one reappears.
  function measure(): void {
    const box = content.value
    rails.value = selectors.map((selector, index) => {
      const row = box?.querySelector<HTMLElement>(selector)
      if (!box || !row) return { ...rails.value[index], shown: false }
      return {
        y: row.getBoundingClientRect().top - box.getBoundingClientRect().top,
        height: row.offsetHeight,
        shown: true,
      }
    })
  }

  let settleUntil = 0
  let settleFrame = 0
  function update(): void {
    measure()
    if (!options.settle?.()) return
    settleUntil = performance.now() + SETTLE_MS
    if (settleFrame) return
    const step = (): void => {
      measure()
      settleFrame = performance.now() < settleUntil ? requestAnimationFrame(step) : 0
    }
    settleFrame = requestAnimationFrame(step)
  }

  useResizeObserver(content, update)
  if (options.sources) watch(options.sources, () => void nextTick(update), { immediate: true })
  onScopeDispose(() => {
    if (settleFrame) cancelAnimationFrame(settleFrame)
  })

  return { rails, update }
}
