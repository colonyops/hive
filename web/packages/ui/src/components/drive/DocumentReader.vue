<script setup lang="ts">
// One document rendered by kind. The desktop pane and page and the web server
// share it, so a document reads the same everywhere and only the width differs.
import { useStorage } from '@vueuse/core'
import { computed, ref, type ComponentPublicInstance } from 'vue'
import HtmlDocumentFrame from './HtmlDocumentFrame.vue'
import DocumentFrontmatter from './DocumentFrontmatter.vue'
import CanvasMarkdown from '../CanvasMarkdown.vue'
import InlineError from '../ui/InlineError.vue'
import { entryTime, markdownBody, type FrontmatterValue, type ReaderDocument } from '../../lib/document'
import type { SourceLines } from '../../lib/githubMarkdown'

const props = withDefaults(
  defineProps<{
    document: ReaderDocument
    fontSizePx: number
    lineHeight: number
    /** Loads the html document from this URL instead of inlining it as `srcdoc`. */
    htmlSrc?: string
    /** Derives `-document`, `-markdown`, `-frame`, and `-error`. */
    testid?: string
    error?: string | null
    /** Marks each markdown block with its source lines (renderCanvasMarkdown). */
    sourceLines?: SourceLines
    bodyClass?: string | Record<string, boolean>
  }>(),
  { htmlSrc: undefined, testid: 'document', error: null, sourceLines: undefined, bodyClass: undefined },
)
const emit = defineEmits<{ (e: 'open-link', target: string): void }>()

const readerStyle = computed(() => ({
  '--hv-font-size': `${props.fontSizePx}px`,
  '--hv-line-height': String(props.lineHeight),
}))

// A host that draws over the rendered markdown (the desktop's review
// highlights) reads these and fills the default slot.
const root = ref<HTMLElement | null>(null)
const markdownView = ref<ComponentPublicInstance | null>(null)
const body = computed(() => (markdownView.value?.$el as HTMLElement | undefined) ?? null)
defineExpose({ root, body })

const propertiesExpanded = useStorage('hive.canvas.frontmatter.expanded', true)

const kind = computed(() => props.document.entry.kind)

const properties = computed<{ frontmatter: Record<string, FrontmatterValue>; created: number; updated: number } | null>(
  () => {
    const doc = props.document
    if (doc.entry.kind !== 'markdown') return null
    return {
      frontmatter: doc.entry.front ?? {},
      created: entryTime(doc.entry.created ?? ''),
      updated: entryTime(doc.entry.modified ?? ''),
    }
  },
)
</script>

<template>
  <div
    ref="root"
    class="document-reader isolate flex min-h-0 flex-col"
    :class="kind === 'html' ? 'absolute inset-0' : 'relative'"
    :style="readerStyle"
    :data-testid="`${testid}-document`"
  >
    <DocumentFrontmatter
      v-if="properties"
      :frontmatter="properties.frontmatter"
      :created-at="properties.created"
      :updated-at="properties.updated"
      :testid="testid"
      :expanded="propertiesExpanded"
      @toggle="propertiesExpanded = !propertiesExpanded"
    />
    <InlineError v-if="error" :testid="`${testid}-error`" variant="line" class="leading-relaxed" :message="error" />
    <CanvasMarkdown
      v-else-if="kind === 'markdown'"
      ref="markdownView"
      class="document-body text-text-2 markdown-body"
      :class="bodyClass"
      :style="readerStyle"
      :source="markdownBody(document.markdown ?? '')"
      :source-lines="sourceLines"
      :testid="`${testid}-markdown`"
      :data-testid="`${testid}-markdown`"
      @follow="emit('open-link', $event)"
    />
    <HtmlDocumentFrame v-else-if="kind === 'html'" :html="document.html ?? ''" :src="htmlSrc" :testid="testid" />
    <slot />
  </div>
</template>

<style scoped>
.document-reader {
  --hv-surface-panel: color-mix(in oklab, var(--color-card) 45%, var(--color-raised));
  --hv-surface-border: color-mix(in oklab, var(--color-text) 8%, var(--color-border));
  --hv-surface-rule: var(--color-border);
  --hv-ink-muted: var(--color-text-3);
  font-size: var(--hv-font-size);
  line-height: var(--hv-line-height);
}
.document-body {
  font-size: var(--hv-font-size);
  line-height: var(--hv-line-height);
}
.document-body.markdown-body :deep(h1),
.document-body.markdown-body :deep(h2),
.document-body.markdown-body :deep(h3),
.document-body.markdown-body :deep(h4),
.document-body.markdown-body :deep(h5),
.document-body.markdown-body :deep(h6) {
  line-height: calc(var(--hv-line-height) * 0.79);
}
.document-body.markdown-body :deep(h1) {
  font-size: 1.407em;
}
.document-body.markdown-body :deep(h2) {
  font-size: 1.222em;
}
.document-body.markdown-body :deep(h3) {
  font-size: 1.111em;
}
.document-body.markdown-body :deep(h4),
.document-body.markdown-body :deep(h5),
.document-body.markdown-body :deep(h6) {
  font-size: 1.037em;
}
.document-body.markdown-body :deep(pre) {
  line-height: calc(var(--hv-line-height) * 0.91);
}
.document-body.markdown-body :deep(:not(pre) > code) {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--hv-surface-panel);
  color: var(--color-text);
  font-size: 0.893em;
}
.document-body.markdown-body :deep(th),
.document-body.markdown-body :deep(td) {
  padding: 7px 14px 7px 0;
  border: 0;
  border-bottom: 1px solid var(--hv-surface-rule);
  background: transparent;
}
.document-body.markdown-body :deep(th) {
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
.document-body.markdown-body :deep(tr) {
  background: transparent;
}
</style>
