<script setup lang="ts">
// A handle in the gutter beside the block under the pointer. It selects that
// whole block, which opens the composer the way any selection does. Place it
// in DocumentReader's slot, beside the highlight layer.
import { useEventListener, useTimeoutFn } from '@vueuse/core'
import { ref, shallowRef } from 'vue'
import IconMessageSquarePlus from '~icons/lucide/message-square-plus'
import { blockTextRange, pickableBlock, type HighlightBox } from '../../lib/reviewSelection'

const props = defineProps<{
  /** DocumentReader's root, the box the handle is placed in. */
  root: HTMLElement | null
  body: HTMLElement | null
  /** False hides the handle, as while a draft is open. */
  enabled: boolean
  /** Derives `-block-comment`. */
  testid: string
}>()

const handleSize = 24
const handleGap = 10

const picked = shallowRef<HTMLElement | null>(null)
const handle = ref<{ top: number; left: number } | null>(null)
const preview = ref<HighlightBox | null>(null)

function point(block: HTMLElement | null): void {
  picked.value = block
  preview.value = null
  if (!block || !props.root) {
    handle.value = null
    return
  }
  const base = props.root.getBoundingClientRect()
  const rect = block.getBoundingClientRect()
  const lineHeight = Number.parseFloat(getComputedStyle(block).lineHeight) || handleSize
  const firstLine = block.matches('.markdown-code, table, .canvas-fence') ? handleSize + 8 : lineHeight
  // The reader's own padding is the gutter; a narrow pane's is thinner than
  // the handle, which then overlaps the text's edge rather than clips.
  const gutter = base.left - (props.root.parentElement?.getBoundingClientRect().left ?? base.left)
  // Every handle sits in one column left of the text, not beside its own
  // block, so a list item's handle stays clear of the item's marker.
  const textLeft = props.body?.getBoundingClientRect().left ?? rect.left
  handle.value = {
    top: rect.top - base.top + (Math.min(rect.height, firstLine) - handleSize) / 2,
    left: Math.max(textLeft - base.left - handleSize - handleGap, -gutter),
  }
}

// Moving left from the text to the handle crosses space that belongs to no
// block, or to an enclosing list's indent: while the pointer stays on the
// picked block's rows, that block stays picked.
function onPickedRows(event: MouseEvent): boolean {
  const rect = picked.value?.getBoundingClientRect()
  return !!rect && event.clientY >= rect.top && event.clientY <= rect.bottom && event.clientX <= rect.right
}

// The handle sits in the gutter, partly outside the reader, so leaving the
// reader on the way to it hides it only after a pause the handle cancels.
const hide = useTimeoutFn(() => point(null), 300, { immediate: false })

useEventListener(
  () => props.root,
  'mousemove',
  (event: MouseEvent) => {
    hide.stop()
    if (!props.enabled || !props.body || event.buttons) {
      if (picked.value) point(null)
      return
    }
    if (event.target instanceof Element && event.target.closest('[data-block-comment-handle]')) return
    const block = pickableBlock(event.target, props.body)
    if (block === picked.value) return
    if ((!block || block.contains(picked.value)) && onPickedRows(event)) return
    point(block)
  },
)
useEventListener(
  () => props.root,
  'mouseleave',
  () => hide.start(),
)

function showPreview(on: boolean): void {
  const block = picked.value
  if (!on || !block || !props.root) {
    preview.value = null
    return
  }
  const base = props.root.getBoundingClientRect()
  const rect = block.getBoundingClientRect()
  preview.value = { top: rect.top - base.top, left: rect.left - base.left, width: rect.width, height: rect.height }
}

function select(): void {
  const block = picked.value
  const range = block ? blockTextRange(block) : null
  const selection = globalThis.getSelection()
  if (!range || !selection) return
  selection.removeAllRanges()
  selection.addRange(range)
  point(null)
}
</script>

<template>
  <div
    v-if="preview"
    class="review-block-preview"
    :style="{
      top: `${preview.top}px`,
      left: `${preview.left}px`,
      width: `${preview.width}px`,
      height: `${preview.height}px`,
    }"
    aria-hidden="true"
  />
  <button
    v-if="handle && enabled"
    type="button"
    data-block-comment-handle
    class="review-block-handle"
    :style="{ top: `${handle.top}px`, left: `${handle.left}px` }"
    aria-label="Comment on this block"
    :data-testid="`${testid}-block-comment`"
    @mousedown.prevent
    @mouseenter="(hide.stop(), showPreview(true))"
    @mouseleave="showPreview(false)"
    @click="select"
  >
    <IconMessageSquarePlus class="size-3.5" aria-hidden="true" />
  </button>
</template>

<style scoped>
.review-block-preview {
  position: absolute;
  z-index: 10;
  pointer-events: none;
  margin: -0.1875rem 0 0 -0.25rem;
  padding: 0.1875rem 0.25rem;
  box-sizing: content-box;
  border-radius: 4px;
  outline: 1px dashed color-mix(in oklab, var(--color-accent) 70%, transparent);
}
.review-block-handle {
  position: absolute;
  z-index: 20;
  display: inline-flex;
  width: 1.5rem;
  height: 1.5rem;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: 6px;
  background: var(--color-accent-tint);
  color: var(--color-accent);
  box-shadow: 0 0 0 1px color-mix(in oklab, var(--color-accent) 45%, transparent);
}
/* The bridge spans the gap to the text, so the pointer never leaves the
   handle's hover on its way over. */
.review-block-handle::after {
  content: '';
  position: absolute;
  inset: -0.375rem -0.625rem -0.375rem -0.375rem;
}
.review-block-handle:hover {
  background: var(--color-accent);
  color: var(--color-accent-contrast);
}
</style>
