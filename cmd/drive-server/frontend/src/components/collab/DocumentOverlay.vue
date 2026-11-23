<script setup lang="ts">
// Sits in DocumentReader's slot over rendered markdown: thread highlights,
// other viewers' selections, the gutter handle, and the comment popover on a
// selection.
import BlockCommentHandle from '@hive/ui/components/review/BlockCommentHandle.vue'
import CommentPopover from '@hive/ui/components/review/CommentPopover.vue'
import ReviewHighlightLayer from '@hive/ui/components/review/ReviewHighlightLayer.vue'
import { useSelectionDraft } from '@hive/ui/composables/useSelectionDraft'
import {
  highlightAt,
  markReviewedBlocks,
  rangeBoxes,
  reviewHighlights,
  type HighlightBox,
  type ReviewHighlight,
} from '@hive/ui/lib/reviewSelection'
import { useEventListener, useMutationObserver, useResizeObserver } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import type { Anchor, LineRange, PresenceViewer, Thread } from '../../types'

const props = defineProps<{
  root: HTMLElement | null
  body: HTMLElement | null
  threads: readonly Thread[]
  viewers: readonly PresenceViewer[]
  activeId: string | null
  hoveredId: string | null
  canComment: boolean
  /** Posts a new thread and answers an error to show, or null. */
  submit: (anchor: Anchor, body: string) => Promise<string | null>
}>()
const emit = defineEmits<{ selection: [range: LineRange | null]; select: [id: string]; hover: [id: string | null] }>()

const testid = 'collab'
const maxQuote = 2000
const viewerPrefix = 'viewer:'
const isMac = /mac/i.test(globalThis.navigator?.platform ?? '')

const popover = ref<{ contains: (node: Node | null) => boolean } | null>(null)
const draftText = ref('')
const saving = ref(false)
const saveError = ref<string | null>(null)

// Selections are tracked for presence whether or not this viewer can comment.
const draft = useSelectionDraft({
  body: () => props.body,
  enabled: () => !!props.body,
  hold: () => !!popover.value?.contains(globalThis.document.activeElement) || !!draftText.value.trim(),
  wait: 150,
})

watch(draft.range, (range) => {
  saveError.value = null
  emit('selection', range ? { start_line: range.startLine, end_line: range.endLine } : null)
})

function endComment(): void {
  draft.end()
  draftText.value = ''
  saveError.value = null
}

async function save(text: string): Promise<void> {
  const range = draft.range.value
  if (!range) return
  saving.value = true
  saveError.value = await props.submit(
    {
      start_line: range.startLine,
      end_line: range.endLine,
      quote: range.context.replace(/\s+/g, ' ').slice(0, maxQuote),
    },
    text,
  )
  saving.value = false
  if (saveError.value) return
  globalThis.getSelection()?.removeAllRanges()
  endComment()
}

const highlightLayer = ref<{ el: HTMLElement | null } | null>(null)
const layer = computed(() => highlightLayer.value?.el ?? null)
const highlights = ref<ReviewHighlight[]>([])
const pending = ref<HighlightBox[]>([])

const anchored = computed(() =>
  props.threads
    .filter((t) => t.anchor && !t.outdated && !t.resolved_at)
    .map((t) => ({ id: t.id, startLine: t.anchor!.start_line, endLine: t.anchor!.end_line, context: t.anchor!.quote })),
)
// A viewer's selection is lines only, so it paints its whole range.
const others = computed(() =>
  props.viewers
    .filter((v) => !v.self && v.selection)
    .map((v) => ({
      id: viewerPrefix + v.id,
      startLine: v.selection!.start_line,
      endLine: v.selection!.end_line,
      context: '',
    })),
)
const tints = computed(() => new Map(props.viewers.map((v) => [viewerPrefix + v.id, `var(--presence-${v.color})`])))

// One markReviewedBlocks pass for both, because each call clears the last.
function mark(): void {
  if (!props.body) return
  const all = [...anchored.value, ...others.value]
  markReviewedBlocks(props.body, all)
  highlights.value = layer.value ? reviewHighlights(props.body, layer.value, all, 'loose') : []
  pending.value = layer.value && draft.domRange.value ? rangeBoxes(draft.domRange.value, layer.value) : []
}
useMutationObserver(() => props.body, mark, { childList: true, subtree: true })
useResizeObserver(() => props.body, mark)
useResizeObserver(layer, mark)
watch([() => props.body, anchored, others, draft.range], mark, { flush: 'post' })

function threadAt(event: MouseEvent, current: string | null = null): string | null {
  if (!layer.value) return null
  const base = layer.value.getBoundingClientRect()
  return highlightAt(
    highlights.value.filter((h) => !h.id.startsWith(viewerPrefix)),
    event.clientX - base.left,
    event.clientY - base.top,
    current,
  )
}
useEventListener(
  () => props.body,
  'mousemove',
  (event: MouseEvent) => {
    const id = threadAt(event)
    if (id !== props.hoveredId) emit('hover', id)
  },
)
useEventListener(
  () => props.body,
  'mouseleave',
  () => emit('hover', null),
)
useEventListener(
  () => props.body,
  'click',
  (event: MouseEvent) => {
    if (!globalThis.getSelection()?.isCollapsed) return
    if (event.target instanceof Element && event.target.closest('a')) return
    const id = threadAt(event, props.activeId)
    if (id) emit('select', id)
  },
)
</script>

<template>
  <ReviewHighlightLayer
    ref="highlightLayer"
    :highlights="highlights"
    :pending="canComment ? pending : []"
    :tints="tints"
    :hovered="hoveredId"
    :active="activeId"
    :testid="testid"
    data-testid="collab-overlay"
  />
  <BlockCommentHandle :root="root" :body="body" :enabled="canComment && !draft.range.value" :testid="testid" />
  <CommentPopover
    v-if="canComment && draft.range.value && draft.rect.value"
    ref="popover"
    v-model:text="draftText"
    :rect="draft.rect.value"
    :range="draft.range.value"
    :busy="saving"
    :error="saveError"
    :save-key="isMac ? '⌘↵' : 'Ctrl+Enter'"
    :testid="testid"
    @save="save"
    @cancel="endComment"
  />
</template>
