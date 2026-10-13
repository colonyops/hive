<script setup lang="ts">
// One document rendered by kind. The pane and the full-page view share it, so
// a document reads the same in both and only the width differs.
import { useEventListener, useMutationObserver } from '@vueuse/core'
import { computed, ref, watch, type ComponentPublicInstance } from 'vue'
import HtmlDocumentFrame from './HtmlDocumentFrame.vue'
import DriveCommentPopover from './DriveCommentPopover.vue'
import DriveFrontmatter from './DriveFrontmatter.vue'
import CanvasMarkdown from '../CanvasMarkdown.vue'
import EmptyState from '../ui/EmptyState.vue'
import InlineError from '../ui/InlineError.vue'
import IconArrowUpRight from '~icons/lucide/arrow-up-right'
import IconMessageSquarePlus from '~icons/lucide/message-square-plus'
import { useCanvasSettings } from '../../stores/useCanvasSettings'
import { useDriveReview } from '../../stores/useDriveReview'
import { entryTime, frontmatterLineCount, linkTarget, markdownBody, type FrontmatterValue } from '../../lib/drive'
import { markReviewedBlocks, selectionToRange, type SourceRange } from '../../lib/driveReviewSelection'
import type { DocumentWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  document: DocumentWire | null
  /** Every document path in the space, which a relative link can name. */
  paths: ReadonlySet<string>
  loading: boolean
  error: string | null
  /**
   * Derives `-document`, `-markdown`, `-block-<id>` (with `-html` and `-link`), `-frame`, `-file`, `-error`,
   * `-empty`, `-comment-button`, and the comment popover's ids (DriveCommentPopover).
   */
  testid: string
}>()
const emit = defineEmits<{ 'open-url': [url: string]; open: [path: string] }>()

const { fontSizePx, lineHeight } = useCanvasSettings()
const readerStyle = computed(() => ({
  '--hv-font-size': `${fontSizePx.value}px`,
  '--hv-line-height': String(lineHeight.value),
}))

const kind = computed(() => props.document?.entry.kind ?? null)
const canvas = computed(() => props.document?.canvas ?? null)
const blocks = computed(() => canvas.value?.blocks ?? [])

const properties = computed<{ frontmatter: Record<string, FrontmatterValue>; created: number; updated: number } | null>(
  () => {
    const doc = props.document
    if (!doc) return null
    if (doc.canvas && doc.canvas.createdAt > 0) {
      return {
        frontmatter: doc.canvas.frontmatter ?? {},
        created: doc.canvas.createdAt,
        updated: doc.canvas.updatedAt,
      }
    }
    if (doc.entry.kind !== 'markdown') return null
    return {
      frontmatter: doc.entry.front ?? {},
      created: entryTime(doc.entry.created),
      updated: entryTime(doc.entry.modified),
    }
  },
)

// Comments: a selection in a Markdown body maps to its source lines, which
// count the front matter the reader shows as properties.
const reviewStore = useDriveReview()
const reviewed = computed(() => {
  const doc = props.document
  return doc && doc.entry.kind === 'markdown' ? { space: doc.space.key, path: doc.entry.path } : null
})
const review = computed(() => (reviewed.value ? reviewStore.review(reviewed.value.space, reviewed.value.path) : null))
const comments = computed(() => review.value?.data.value.comments ?? [])
const current = computed(() => review.value?.data.value.current ?? true)
const sourceLines = computed(() =>
  props.document && kind.value === 'markdown' ? { offset: frontmatterLineCount(props.document.markdown) } : undefined,
)

const markdownView = ref<ComponentPublicInstance | null>(null)
const body = computed(() => (markdownView.value?.$el as HTMLElement | undefined) ?? null)

const selected = ref<SourceRange | null>(null)
const buttonPosition = ref<{ top: number; left: number } | null>(null)
const commentButton = ref<HTMLElement | null>(null)
const drafting = ref<SourceRange | null>(null)
const saving = ref(false)
const saveError = ref<string | null>(null)

useEventListener(
  () => (reviewed.value ? globalThis.document : null),
  'selectionchange',
  () => {
    if (drafting.value) return
    const selection = globalThis.getSelection()
    const range = selection && body.value && current.value ? selectionToRange(selection, body.value) : null
    selected.value = range
    if (!range || !selection) {
      buttonPosition.value = null
      return
    }
    const rect = selection.getRangeAt(0).getBoundingClientRect()
    buttonPosition.value = { top: Math.max(8, rect.top - 34), left: Math.max(8, rect.right - 28) }
  },
)

function startComment(): void {
  drafting.value = selected.value
  saveError.value = null
}

function endComment(): void {
  drafting.value = null
  selected.value = null
  buttonPosition.value = null
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
// renders again), so the markers follow every change to it, not only to the
// comments. The markers set attributes, which this does not observe.
function mark(): void {
  if (body.value) markReviewedBlocks(body.value, current.value ? comments.value : [], reviewStore.active.value)
}
useMutationObserver(body, mark, { childList: true, subtree: true })
watch([body, comments, current, reviewStore.active], mark, { flush: 'post' })

// A click on a commented block points at its comment; a link inside it still
// follows.
useEventListener(body, 'click', (event: MouseEvent) => {
  const target = event.target
  if (!(target instanceof Element) || target.closest('a') || !globalThis.getSelection()?.isCollapsed) return
  const ids = target.closest<HTMLElement>('[data-review-ids]')?.dataset.reviewIds
  if (ids) reviewStore.setActive(ids.split(' ')[0])
})

function follow(href: string): void {
  const target = linkTarget(href, props.document?.entry.path ?? '', props.paths)
  if (target?.kind === 'url') emit('open-url', target.url)
  else if (target?.kind === 'document') emit('open', target.path)
}

// Links must open in the user's real browser rather than navigate the webview
// away from the app: DetailPane's interception, for the same reason.
function onBodyClick(event: MouseEvent): void {
  const anchor = (event.target as HTMLElement).closest('a')
  if (!anchor) return
  event.preventDefault()
  follow(anchor.getAttribute('href') ?? '')
}
</script>

<template>
  <div class="canvas-reader flex min-h-0 flex-col" :style="readerStyle" :data-testid="`${testid}-document`">
    <DriveFrontmatter
      v-if="properties"
      :frontmatter="properties.frontmatter"
      :created-at="properties.created"
      :updated-at="properties.updated"
      :testid="testid"
    />
    <InlineError v-if="error" :testid="`${testid}-error`" variant="line" class="leading-relaxed" :message="error" />
    <CanvasMarkdown
      v-else-if="document && kind === 'markdown'"
      ref="markdownView"
      class="canvas-reading-body text-text-2 markdown-body"
      :style="readerStyle"
      :source="markdownBody(document.markdown)"
      :source-lines="sourceLines"
      :testid="`${testid}-markdown`"
      :data-testid="`${testid}-markdown`"
      @follow="follow"
    />
    <HtmlDocumentFrame v-else-if="document && kind === 'html'" :html="document.html" :testid="testid" />
    <template v-else-if="kind === 'canvas' && blocks.length">
      <article
        v-for="block in blocks"
        :key="block.id"
        class="canvas-block"
        :data-testid="`${testid}-block-${block.id}`"
      >
        <template v-if="block.kind === 'markdown' || block.kind === 'html'">
          <h2 v-if="block.title" class="canvas-block-title">{{ block.title }}</h2>
          <CanvasMarkdown
            v-if="block.kind === 'markdown'"
            class="canvas-reading-body text-text-2 markdown-body"
            :style="readerStyle"
            :source="block.body"
            :testid="`${testid}-block-${block.id}`"
            @follow="follow"
          />
          <!-- eslint-disable vue/no-v-html -- html blocks arrive sanitized by canvas.SanitizeHTML in Go -->
          <div
            v-else
            class="canvas-reading-body text-text-2 hv-html"
            :style="readerStyle"
            :data-testid="`${testid}-block-${block.id}-html`"
            @click="onBodyClick"
            v-html="block.body"
          />
          <!-- eslint-enable vue/no-v-html -->
        </template>
        <button
          v-else-if="block.kind === 'link'"
          type="button"
          class="canvas-link"
          :title="block.url"
          :data-testid="`${testid}-block-${block.id}-link`"
          @click="follow(block.url)"
        >
          <span class="canvas-link-text">
            <span class="canvas-link-title">{{ block.title }}</span>
            <span class="canvas-link-url">{{ block.url }}</span>
          </span>
          <IconArrowUpRight class="canvas-link-arrow" aria-hidden="true" />
        </button>
      </article>
    </template>
    <EmptyState
      v-else-if="document && kind === 'file'"
      variant="inline"
      message="Hive shows markdown, html, and canvas documents. Open this file in another app to read it."
      :data-testid="`${testid}-file`"
    />
    <EmptyState v-else-if="!loading" variant="inline" message="Nothing here yet." :data-testid="`${testid}-empty`" />
    <Teleport to="body">
      <button
        v-if="buttonPosition"
        ref="commentButton"
        type="button"
        class="drive-comment-button fixed z-40 flex h-7 cursor-pointer items-center gap-1 rounded-lg border border-card bg-raised px-2 text-small font-medium text-text shadow-popover hover:bg-chip"
        :style="{ top: `${buttonPosition.top}px`, left: `${buttonPosition.left}px` }"
        :data-testid="`${testid}-comment-button`"
        @mousedown.prevent
        @click="startComment"
      >
        <IconMessageSquarePlus class="size-3.5" aria-hidden="true" />
        Comment
      </button>
    </Teleport>
    <DriveCommentPopover
      v-if="drafting"
      :anchor="commentButton"
      :range="drafting"
      :busy="saving"
      :error="saveError"
      :testid="testid"
      @save="saveComment"
      @cancel="endComment"
    />
  </div>
</template>

<style scoped>
.canvas-reader {
  font-size: var(--hv-font-size);
  line-height: var(--hv-line-height);
}
.canvas-block {
  padding: 12px 0;
}
.canvas-block + .canvas-block {
  border-top: 1px solid var(--color-border);
}
.canvas-block:first-child {
  padding-top: 0;
}
/* A block title is an eyebrow over its body, in the voice of the app's
   section headers, so it never competes with the body's own headings. */
.canvas-block-title {
  margin-bottom: 10px;
  color: var(--hv-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.786em;
  font-weight: 500;
  line-height: 1;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.canvas-reading-body {
  font-size: var(--hv-font-size);
  line-height: var(--hv-line-height);
}
.canvas-reading-body.markdown-body :deep(h1),
.canvas-reading-body.markdown-body :deep(h2),
.canvas-reading-body.markdown-body :deep(h3),
.canvas-reading-body.markdown-body :deep(h4),
.canvas-reading-body.markdown-body :deep(h5),
.canvas-reading-body.markdown-body :deep(h6) {
  line-height: calc(var(--hv-line-height) * 0.79);
}
.canvas-reading-body.markdown-body :deep(h1) {
  font-size: 1.407em;
}
.canvas-reading-body.markdown-body :deep(h2) {
  font-size: 1.222em;
}
.canvas-reading-body.markdown-body :deep(h3) {
  font-size: 1.111em;
}
.canvas-reading-body.markdown-body :deep(h4),
.canvas-reading-body.markdown-body :deep(h5),
.canvas-reading-body.markdown-body :deep(h6) {
  font-size: 1.037em;
}
.canvas-reading-body.markdown-body :deep(pre) {
  line-height: calc(var(--hv-line-height) * 0.91);
}
.canvas-reading-body.markdown-body :deep(:not(pre) > code) {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--hv-surface-panel);
  color: var(--color-text);
  font-size: 0.893em;
}
.canvas-reading-body.markdown-body :deep(th),
.canvas-reading-body.markdown-body :deep(td) {
  padding: 7px 14px 7px 0;
  border: 0;
  border-bottom: 1px solid var(--hv-surface-rule);
  background: transparent;
}
.canvas-reading-body.markdown-body :deep(th) {
  padding-top: 6px;
  padding-bottom: 6px;
  border-bottom-color: var(--hv-surface-border);
  color: var(--hv-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.846em;
  font-weight: 500;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.canvas-reading-body.markdown-body :deep(tr) {
  background: transparent;
}
.canvas-reading-body.markdown-body :deep(.has-review-comment) {
  border-radius: 4px;
  background: var(--color-accent-tint);
  box-shadow: -6px 0 0 var(--color-accent-tint);
}
.canvas-reading-body.markdown-body :deep(.has-review-comment) {
  cursor: pointer;
}
.canvas-reading-body.markdown-body :deep(.is-active-review) {
  background: color-mix(in oklab, var(--color-accent) 22%, transparent);
  box-shadow: -6px 0 0 color-mix(in oklab, var(--color-accent) 22%, transparent);
}
.canvas-reading-body.markdown-body :deep([data-review-marker]) {
  position: relative;
}
.canvas-reading-body.markdown-body :deep([data-review-marker])::before {
  /* Inside the block's right edge: the pane's narrow gutter would clip a
     badge in the left margin. */
  position: absolute;
  top: 0;
  right: 0;
  min-width: 18px;
  padding: 0 4px;
  border-radius: 9px;
  background: var(--color-accent);
  color: var(--color-accent-contrast);
  content: attr(data-review-marker);
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 18px;
  text-align: center;
}
.canvas-link {
  display: flex;
  width: 100%;
  min-width: 0;
  cursor: pointer;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--hv-surface-border);
  border-radius: 8px;
  background: var(--hv-surface-card);
  box-shadow: var(--hv-surface-shadow);
  color: var(--color-text);
  text-align: left;
  transition: background-color 150ms ease;
}
.canvas-link:hover,
.canvas-link:focus-visible {
  background: color-mix(in oklab, var(--hv-surface-panel) 60%, var(--hv-surface-card));
}
.canvas-link-text {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 2px;
}
.canvas-link-title {
  font-weight: 500;
  line-height: 1.4;
}
.canvas-link-url {
  overflow: hidden;
  color: var(--hv-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.857em;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.canvas-link-arrow {
  flex-shrink: 0;
  color: var(--hv-ink-muted);
  transition: color 150ms ease;
}
.canvas-link:hover .canvas-link-arrow,
.canvas-link:focus-visible .canvas-link-arrow {
  color: var(--color-text);
}
</style>
