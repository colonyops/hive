<script setup lang="ts">
import { useDebounceFn, useEventListener, useMutationObserver, useResizeObserver, useTimeoutFn } from '@vueuse/core'
import { computed, ref, shallowRef, watch } from 'vue'
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { frontmatterLineCount, linkTarget } from '@hive/ui/lib/document'
import DriveCommentPopover from './DriveCommentPopover.vue'
import IconMessageSquarePlus from '~icons/lucide/message-square-plus'
import { useCanvasSettings } from '../../stores/useCanvasSettings'
import { useDriveReview } from '../../stores/useDriveReview'
import { toReaderDocument } from '../../lib/drive'
import {
  blockTextRange,
  highlightAt,
  markReviewedBlocks,
  pickableBlock,
  rangeBoxes,
  reviewHighlights,
  type HighlightBox,
  selectionToRange,
  type ReviewHighlight,
  type SourceRange,
} from '../../lib/driveReviewSelection'
import type { DocumentWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  document: DocumentWire | null
  /** Every document path in the space, which a relative link can name. */
  paths: ReadonlySet<string>
  loading: boolean
  error: string | null
  /**
   * Derives `-document`, `-markdown`, `-block-<id>` (with `-html` and `-link`), `-frame`, `-file`, `-error`,
   * `-empty`, `-block-comment`, and the comment popover's ids (DriveCommentPopover).
   */
  testid: string
}>()
const emit = defineEmits<{ 'open-url': [url: string]; open: [path: string] }>()

const { fontSizePx, lineHeight } = useCanvasSettings()
const readerDocument = computed(() => (props.document ? toReaderDocument(props.document) : null))
const bareStyle = computed(() => ({ fontSize: `${fontSizePx.value}px`, lineHeight: String(lineHeight.value) }))

function follow(href: string): void {
  const target = linkTarget(href, props.document?.entry.path ?? '', props.paths)
  if (target?.kind === 'url') emit('open-url', target.url)
  else if (target?.kind === 'document') emit('open', target.path)
}

// Source lines count the front matter, which the reader shows as properties.
const reviewStore = useDriveReview()
const reviewed = computed(() => {
  const doc = props.document
  return doc && doc.entry.kind === 'markdown' ? { space: doc.space.key, path: doc.entry.path } : null
})
const review = computed(() => (reviewed.value ? reviewStore.review(reviewed.value.space, reviewed.value.path) : null))
const comments = computed(() => review.value?.data.value.comments ?? [])
const current = computed(() => review.value?.data.value.current ?? true)
const sourceLines = computed(() =>
  reviewed.value && props.document ? { offset: frontmatterLineCount(props.document.markdown) } : undefined,
)

// What DocumentReader exposes. Spelled out because ESLint cannot resolve a
// .vue component's instance type.
const reader = ref<{ root: HTMLElement | null; body: HTMLElement | null } | null>(null)
const root = computed(() => reader.value?.root ?? null)
const body = computed(() => reader.value?.body ?? null)

const drafting = ref<SourceRange | null>(null)
// The selection a draft is for, painted as a pending highlight: clicking into
// the composer collapses the live selection, and the text it was about must
// stay marked.
let draftRange: Range | null = null
const pending = ref<HighlightBox[]>([])
const draftText = ref('')
const anchorRect = ref<{ top: number; left: number; width: number; height: number } | null>(null)
const popover = ref<{ contains: (node: Node | null) => boolean } | null>(null)
const saving = ref(false)
const saveError = ref<string | null>(null)

const onSelectionChange = useDebounceFn(() => {
  if (popover.value?.contains(globalThis.document.activeElement) || draftText.value.trim()) return
  const selection = globalThis.getSelection()
  const range = selection && body.value && current.value ? selectionToRange(selection, body.value) : null
  if (!range || !selection) {
    endComment()
    return
  }
  const selected = selection.getRangeAt(0)
  const rect = selected.getBoundingClientRect()
  anchorRect.value = { top: rect.top, left: rect.left, width: rect.width, height: rect.height }
  draftRange = selected.cloneRange()
  drafting.value = range
  saveError.value = null
}, 200)

useEventListener(
  () => (reviewed.value ? globalThis.document : null),
  'selectionchange',
  () => void onSelectionChange(),
)

function endComment(): void {
  draftRange = null
  pending.value = []
  drafting.value = null
  draftText.value = ''
  anchorRect.value = null
  saveError.value = null
}

async function saveComment(text: string): Promise<void> {
  const target = reviewed.value
  const range = drafting.value
  if (!target || !range) return
  saving.value = true
  const error = await reviewStore.add(target.space, target.path, range, text)
  saving.value = false
  if (error) {
    saveError.value = error
    return
  }
  globalThis.getSelection()?.removeAllRanges()
  endComment()
}

watch(reviewed, endComment)

// Rendering replaces the body's markup (a highlighter that finishes loading
// renders again), so the marks follow every change to it, not only to the
// comments. The marks set attributes, which this does not observe.
const layer = ref<HTMLElement | null>(null)
const highlights = ref<ReviewHighlight[]>([])
const shownComments = computed(() => (current.value ? comments.value : []))

function mark(): void {
  if (!body.value) return
  markReviewedBlocks(body.value, shownComments.value)
  highlights.value = layer.value ? reviewHighlights(body.value, layer.value, shownComments.value) : []
  pending.value = layer.value && draftRange ? rangeBoxes(draftRange, layer.value) : []
}
useMutationObserver(body, mark, { childList: true, subtree: true })
useResizeObserver(body, mark)
// The layer spans the whole reader, so it also resizes when the front matter
// above the body opens or closes and moves the body without resizing it.
useResizeObserver(layer, mark)
watch([body, shownComments, drafting], mark, { flush: 'post' })

// Hover and click hit the drawn highlights, not the blocks under them, so the
// rest of a paragraph with a one-word comment stays plain text.
function commentAtPoint(event: MouseEvent, current: string | null = null): string | null {
  if (!layer.value) return null
  const base = layer.value.getBoundingClientRect()
  return highlightAt(highlights.value, event.clientX - base.left, event.clientY - base.top, current)
}
useEventListener(body, 'mousemove', (event: MouseEvent) => reviewStore.setHovered(commentAtPoint(event)))
useEventListener(body, 'mouseleave', () => reviewStore.setHovered(null))

useEventListener(body, 'click', (event: MouseEvent) => {
  const target = event.target
  if (!(target instanceof Element) || target.closest('a') || !globalThis.getSelection()?.isCollapsed) return
  const id = commentAtPoint(event, reviewStore.active.value)
  if (id) reviewStore.setActive(id)
})

// A handle in the gutter beside the block under the pointer selects that
// whole block, which opens the composer the way any selection does.
const pickedBlock = shallowRef<HTMLElement | null>(null)
const handleBox = ref<{ top: number; left: number } | null>(null)
const blockPreview = ref<HighlightBox | null>(null)
const handleSize = 24
const handleGap = 10

function pointBlock(block: HTMLElement | null): void {
  pickedBlock.value = block
  blockPreview.value = null
  if (!block || !root.value) {
    handleBox.value = null
    return
  }
  const base = root.value.getBoundingClientRect()
  const rect = block.getBoundingClientRect()
  const lineHeight = Number.parseFloat(getComputedStyle(block).lineHeight) || handleSize
  const firstLine = block.matches('.markdown-code, table, .canvas-mermaid') ? handleSize + 8 : lineHeight
  // The reader's own padding is the gutter; the narrow pane's is thinner
  // than the handle, which then overlaps the text's edge rather than clips.
  const gutter = base.left - (root.value.parentElement?.getBoundingClientRect().left ?? base.left)
  handleBox.value = {
    top: rect.top - base.top + (Math.min(rect.height, firstLine) - handleSize) / 2,
    left: Math.max(rect.left - base.left - handleSize - handleGap, -gutter),
  }
}

// Moving left from the text to the handle crosses space that belongs to no
// block, or to an enclosing list's indent: while the pointer stays on the
// picked block's rows, that block stays picked.
function onPickedRows(event: MouseEvent): boolean {
  const rect = pickedBlock.value?.getBoundingClientRect()
  return !!rect && event.clientY >= rect.top && event.clientY <= rect.bottom && event.clientX <= rect.right
}

// The handle sits in the gutter, partly outside the reader, so leaving the
// reader on the way to it hides it only after a pause the handle cancels.
const hideHandle = useTimeoutFn(() => pointBlock(null), 300, { immediate: false })

useEventListener(root, 'mousemove', (event: MouseEvent) => {
  hideHandle.stop()
  if (!body.value || !current.value || drafting.value || event.buttons) {
    if (pickedBlock.value) pointBlock(null)
    return
  }
  if (event.target instanceof Element && event.target.closest('[data-block-comment-handle]')) return
  const block = pickableBlock(event.target, body.value)
  if (block === pickedBlock.value) return
  if ((!block || block.contains(pickedBlock.value)) && onPickedRows(event)) return
  pointBlock(block)
})
useEventListener(root, 'mouseleave', () => hideHandle.start())

function previewBlock(on: boolean): void {
  const block = pickedBlock.value
  if (!on || !block || !layer.value) {
    blockPreview.value = null
    return
  }
  const base = layer.value.getBoundingClientRect()
  const rect = block.getBoundingClientRect()
  blockPreview.value = { top: rect.top - base.top, left: rect.left - base.left, width: rect.width, height: rect.height }
}

function commentOnBlock(): void {
  const block = pickedBlock.value
  const range = block ? blockTextRange(block) : null
  const selection = globalThis.getSelection()
  if (!range || !selection) return
  selection.removeAllRanges()
  selection.addRange(range)
  pointBlock(null)
}
</script>

<template>
  <DocumentReader
    v-if="readerDocument"
    ref="reader"
    :document="readerDocument"
    :font-size-px="fontSizePx"
    :line-height="lineHeight"
    :testid="testid"
    :error="error"
    :loading="loading"
    :source-lines="sourceLines"
    :body-class="{ 'cursor-pointer': !!reviewStore.hovered.value }"
    @open-link="follow"
  >
    <div ref="layer" class="pointer-events-none absolute inset-0 z-10" aria-hidden="true">
      <template v-for="highlight in highlights" :key="highlight.id">
        <div
          v-for="(box, index) in highlight.boxes"
          :key="index"
          class="review-highlight"
          :class="{
            'is-hovered': reviewStore.hovered.value === highlight.id,
            'is-active': reviewStore.active.value === highlight.id,
          }"
          :style="{ top: `${box.top}px`, left: `${box.left}px`, width: `${box.width}px`, height: `${box.height}px` }"
          :data-review-anchor="index === 0 ? highlight.id : undefined"
          :data-testid="`${testid}-highlight-${highlight.id}`"
        />
      </template>
      <div
        v-for="(box, index) in pending"
        :key="`pending-${index}`"
        class="review-highlight is-pending"
        :style="{ top: `${box.top}px`, left: `${box.left}px`, width: `${box.width}px`, height: `${box.height}px` }"
        :data-testid="`${testid}-highlight-pending`"
      />
      <div
        v-if="blockPreview"
        class="review-block-preview"
        :style="{
          top: `${blockPreview.top}px`,
          left: `${blockPreview.left}px`,
          width: `${blockPreview.width}px`,
          height: `${blockPreview.height}px`,
        }"
      />
    </div>
    <button
      v-if="handleBox"
      type="button"
      data-block-comment-handle
      class="review-block-handle"
      :style="{ top: `${handleBox.top}px`, left: `${handleBox.left}px` }"
      aria-label="Comment on this block"
      :data-testid="`${testid}-block-comment`"
      @mousedown.prevent
      @mouseenter="(hideHandle.stop(), previewBlock(true))"
      @mouseleave="previewBlock(false)"
      @click="commentOnBlock"
    >
      <IconMessageSquarePlus class="size-3.5" aria-hidden="true" />
    </button>
    <DriveCommentPopover
      v-if="drafting && anchorRect"
      ref="popover"
      v-model:text="draftText"
      :rect="anchorRect"
      :range="drafting"
      :busy="saving"
      :error="saveError"
      :testid="testid"
      @save="saveComment"
      @cancel="endComment"
    />
  </DocumentReader>
  <div v-else class="flex min-h-0 flex-col" :style="bareStyle" :data-testid="`${testid}-document`">
    <InlineError v-if="error" :testid="`${testid}-error`" variant="line" class="leading-relaxed" :message="error" />
    <EmptyState
      v-else-if="document"
      variant="inline"
      message="Hive shows markdown, html, and canvas documents. Open this file in another app to read it."
      :data-testid="`${testid}-file`"
    />
    <EmptyState v-else-if="!loading" variant="inline" message="Nothing here yet." :data-testid="`${testid}-empty`" />
  </div>
</template>

<style scoped>
/* The highlight sits over the text rather than behind it because inline code
   and other chips paint opaque backgrounds that would cut holes in it, so its
   tint stays light enough to read through. */
.review-highlight {
  position: absolute;
  margin-left: -2px;
  padding: 0 2px;
  box-sizing: content-box;
  border-radius: 2px;
  background: color-mix(in oklab, var(--color-accent) 16%, transparent);
  transition:
    background-color 120ms ease,
    box-shadow 120ms ease;
}
.review-highlight.is-hovered {
  background: color-mix(in oklab, var(--color-accent) 24%, transparent);
}
.review-highlight.is-active {
  background: color-mix(in oklab, var(--color-accent) 28%, transparent);
  box-shadow: 0 0 0 1px color-mix(in oklab, var(--color-accent) 55%, transparent);
}
.review-highlight.is-pending {
  background: color-mix(in oklab, var(--color-accent) 28%, transparent);
}
.review-block-preview {
  position: absolute;
  margin: -3px 0 0 -4px;
  padding: 3px 4px;
  box-sizing: content-box;
  border-radius: 4px;
  outline: 1px dashed color-mix(in oklab, var(--color-accent) 70%, transparent);
}
.review-block-handle {
  position: absolute;
  z-index: 20;
  display: inline-flex;
  width: 24px;
  height: 24px;
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
  inset: -6px -10px -6px -6px;
}
.review-block-handle:hover {
  background: var(--color-accent);
  color: var(--color-accent-contrast);
}
</style>
