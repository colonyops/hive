import { useMutationObserver, useResizeObserver } from '@vueuse/core'
import { nextTick, ref, toValue, watch, type MaybeRefOrGetter, type Ref, type WatchSource } from 'vue'

export interface AlignedCardsOptions {
  /** The cards' own box. It must scroll with the document, so scrolling never moves a card. */
  cards: Readonly<Ref<HTMLElement | null>>
  ids: MaybeRefOrGetter<readonly string[]>
  /** The element a card lines up with, or null to stack it under the card above. */
  anchor: (id: string) => HTMLElement | null
  /** The element holding the document; null stacks every card. */
  alignTo: MaybeRefOrGetter<HTMLElement | null>
  /** Anything else that changes a card's height or its anchor. */
  deps?: WatchSource[]
  gap?: number
}

/**
 * Places each card on the row of the text it is about, sliding it down only
 * to clear the card above. Spread `cardRef` onto each card and position it at
 * `tops.get(id)`; give the cards' box a min-height of `height`.
 */
export function useAlignedCards(opts: AlignedCardsOptions) {
  const gap = opts.gap ?? 8
  const elements = new Map<string, HTMLElement>()
  const tops = ref<ReadonlyMap<string, number>>(new Map())
  const height = ref(0)
  // Only a card already on its anchor slides. One that just mounted, or that
  // waited at a fallback slot while a newly opened document rendered, would
  // otherwise fly across the column to its first real position.
  let anchored = new Set<string>()
  const sliding = ref<ReadonlySet<string>>(new Set())

  /** Takes the card's element, or a component whose root is the card. */
  function cardRef(id: string, card: unknown): void {
    const element = card instanceof HTMLElement ? card : (card as { $el?: unknown } | null)?.$el
    if (element instanceof HTMLElement) elements.set(id, element)
    else elements.delete(id)
  }

  function layout(): void {
    const box = opts.cards.value
    if (!toValue(opts.alignTo) || !box) return
    const base = box.getBoundingClientRect().top
    const next = new Map<string, number>()
    const nextAnchored = new Set<string>()
    let floor = 0
    for (const id of toValue(opts.ids)) {
      const target = opts.anchor(id)
      if (target) nextAnchored.add(id)
      const wanted = target ? target.getBoundingClientRect().top - base : floor
      const top = Math.max(wanted, floor)
      next.set(id, top)
      floor = top + (elements.get(id)?.offsetHeight ?? 0) + gap
    }
    sliding.value = new Set([...anchored].filter((id) => nextAnchored.has(id)))
    anchored = nextAnchored
    tops.value = next
    height.value = floor
  }

  function relayout(): void {
    void nextTick(layout)
  }

  watch([() => toValue(opts.ids), () => toValue(opts.alignTo), ...(opts.deps ?? [])], relayout, {
    immediate: true,
    flush: 'post',
  })
  useResizeObserver(() => toValue(opts.alignTo), relayout)
  useResizeObserver(opts.cards, relayout)
  // The reader marks blocks after it renders, and a highlighter that finishes
  // loading renders again.
  useMutationObserver(() => toValue(opts.alignTo), relayout, {
    subtree: true,
    childList: true,
    attributes: true,
    attributeFilter: ['data-review-ids', 'style'],
  })

  return { tops, height, sliding, cardRef, relayout }
}
