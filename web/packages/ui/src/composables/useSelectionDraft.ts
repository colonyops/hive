import { useDebounceFn, useEventListener, useMutationObserver } from '@vueuse/core'
import { ref, shallowRef, toValue, type MaybeRefOrGetter } from 'vue'
import { selectionToRange, type HighlightBox, type SourceRange } from '../lib/reviewSelection'

export interface SelectionDraftOptions {
  /** The rendered markdown the selection must fall in. */
  body: MaybeRefOrGetter<HTMLElement | null>
  enabled: MaybeRefOrGetter<boolean>
  /** True while a draft must outlive a selection change: its composer has focus or text. */
  hold?: () => boolean
  wait?: number
}

/**
 * Follows the document selection inside a rendered markdown body and turns it
 * into the source lines a comment would anchor to. `rect` is the selection's
 * box in the viewport, for placing a composer beside it.
 */
export function useSelectionDraft(opts: SelectionDraftOptions) {
  const range = ref<SourceRange | null>(null)
  const rect = ref<HighlightBox | null>(null)
  // The live selection collapses when the composer takes focus, so the DOM
  // range is kept to paint the text the draft is about.
  const domRange = shallowRef<Range | null>(null)

  function end(): void {
    range.value = null
    rect.value = null
    domRange.value = null
  }

  const refresh = useDebounceFn(() => {
    if (opts.hold?.()) return
    const body = toValue(opts.body)
    const selection = globalThis.getSelection()
    const next = selection && body && toValue(opts.enabled) ? selectionToRange(selection, body) : null
    if (!next || !selection) {
      end()
      return
    }
    const selected = selection.getRangeAt(0)
    const box = selected.getBoundingClientRect()
    rect.value = { top: box.top, left: box.left, width: box.width, height: box.height }
    domRange.value = selected.cloneRange()
    range.value = next
  }, opts.wait ?? 200)

  useEventListener(
    () => (toValue(opts.enabled) ? globalThis.document : null),
    'selectionchange',
    () => void refresh(),
  )
  // A new render replaces the text under a selection without firing
  // selectionchange, and the lines it covers may have moved.
  useMutationObserver(
    () => toValue(opts.body),
    () => void refresh(),
    {
      childList: true,
      subtree: true,
      characterData: true,
    },
  )

  return { range, rect, domRange, end, refresh }
}
