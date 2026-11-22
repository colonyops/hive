<script setup lang="ts">
import { useEventListener, useMutationObserver, useResizeObserver } from '@vueuse/core'
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { frontmatterLineCount, linkTarget } from '@hive/ui/lib/document'
import BlockCommentHandle from '@hive/ui/components/review/BlockCommentHandle.vue'
import ReviewHighlightLayer from '@hive/ui/components/review/ReviewHighlightLayer.vue'
import { useSelectionDraft } from '@hive/ui/composables/useSelectionDraft'
import DriveCommentPopover from './DriveCommentPopover.vue'
import { useCanvasSettings } from '../../stores/useCanvasSettings'
import { useDriveReview } from '../../stores/useDriveReview'
import { toReaderDocument } from '../../lib/drive'
import {
  highlightAt,
  markReviewedBlocks,
  rangeBoxes,
  reviewHighlights,
  type HighlightBox,
  type ReviewHighlight,
} from '@hive/ui/lib/reviewSelection'
import type { DocumentWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

// The diff library and its grammars load with the first text file opened, not with the Drive.
// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- ESLint cannot resolve a .vue module's type
const DriveCodeView = defineAsyncComponent(() => import('./DriveCodeView.vue'))

const props = defineProps<{
  document: DocumentWire | null
  /** Every document path in the space, which a relative link can name. */
  paths: ReadonlySet<string>
  loading: boolean
  error: string | null
  /** Lets the user select text to comment on and shows the document's comments. */
  commentable?: boolean
  /**
   * Derives `-document`, `-markdown`, `-frame`, `-image`, `-code`, `-file`, `-error`,
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
  return props.commentable && doc && doc.entry.kind === 'markdown'
    ? { space: doc.space.key, path: doc.entry.path }
    : null
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

const popover = ref<{ contains: (node: Node | null) => boolean } | null>(null)
const draftText = ref('')
const saving = ref(false)
const saveError = ref<string | null>(null)

const draft = useSelectionDraft({
  body,
  enabled: () => !!reviewed.value && current.value,
  hold: () => !!popover.value?.contains(globalThis.document.activeElement) || !!draftText.value.trim(),
})
const drafting = draft.range

function endComment(): void {
  draft.end()
  draftText.value = ''
  saveError.value = null
}
watch(drafting, () => (saveError.value = null))

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
const highlightLayer = ref<{ el: HTMLElement | null } | null>(null)
const layer = computed(() => highlightLayer.value?.el ?? null)
const highlights = ref<ReviewHighlight[]>([])
const pending = ref<HighlightBox[]>([])
const shownComments = computed(() => (current.value ? comments.value : []))

function mark(): void {
  if (!body.value) return
  markReviewedBlocks(body.value, shownComments.value)
  highlights.value = layer.value ? reviewHighlights(body.value, layer.value, shownComments.value) : []
  pending.value = layer.value && draft.domRange.value ? rangeBoxes(draft.domRange.value, layer.value) : []
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
</script>

<template>
  <DocumentReader
    v-if="readerDocument"
    data-scale-surface="document"
    ref="reader"
    :document="readerDocument"
    :font-size-px="fontSizePx"
    :line-height="lineHeight"
    :testid="testid"
    :error="error"
    :loading="loading"
    :source-lines="sourceLines"
    :paths="paths"
    :body-class="{ 'cursor-pointer': !!reviewStore.hovered.value }"
    @open-link="follow"
  >
    <ReviewHighlightLayer
      ref="highlightLayer"
      :highlights="highlights"
      :pending="pending"
      :hovered="reviewStore.hovered.value"
      :active="reviewStore.active.value"
      :testid="testid"
    />
    <BlockCommentHandle :root="root" :body="body" :enabled="!!reviewed && current && !drafting" :testid="testid" />
    <DriveCommentPopover
      v-if="drafting && draft.rect.value"
      ref="popover"
      v-model:text="draftText"
      :rect="draft.rect.value"
      :range="drafting"
      :busy="saving"
      :error="saveError"
      :testid="testid"
      @save="saveComment"
      @cancel="endComment"
    />
  </DocumentReader>
  <div
    v-else
    class="flex min-h-0 flex-col"
    data-scale-surface="document"
    :style="bareStyle"
    :data-testid="`${testid}-document`"
  >
    <InlineError v-if="error" :testid="`${testid}-error`" variant="line" class="leading-relaxed" :message="error" />
    <img
      v-else-if="document?.image"
      :src="document.image"
      :alt="document.entry.name"
      class="h-auto max-w-full self-center"
      :data-testid="`${testid}-image`"
    />
    <DriveCodeView
      v-else-if="document?.text"
      :id="`${document.space.key}:${document.entry.path}`"
      :name="document.entry.name"
      :text="document.text"
      :version="document.entry.modified"
      :testid="`${testid}-code`"
    />
    <EmptyState
      v-else-if="document"
      variant="inline"
      message="Hive can't show this file. Open it in another app to read it."
      :data-testid="`${testid}-file`"
    />
    <EmptyState v-else-if="!loading" variant="inline" message="Nothing here yet." :data-testid="`${testid}-empty`" />
  </div>
</template>
