<script setup lang="ts">
// One canvas rendered block by block. The pane and the full-page view share
// it, so a canvas reads the same in both and only the width differs.
import { computed } from 'vue'
import AgentCanvasFrontmatter from './AgentCanvasFrontmatter.vue'
import CanvasMarkdown from './CanvasMarkdown.vue'
import EmptyState from './ui/EmptyState.vue'
import InlineError from './ui/InlineError.vue'
import IconArrowUpRight from '~icons/lucide/arrow-up-right'
import { useCanvasSettings } from '../stores/useCanvasSettings'
import { canvasLinkTarget } from '../lib/agentCanvas'
import type { WorkspaceCanvas, WorkspaceCanvasMeta } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  canvas: WorkspaceCanvas | null
  /** The workspace's canvases, which a relative link can name. */
  metas: WorkspaceCanvasMeta[]
  loading: boolean
  error: string
  /** Derives `-block-<id>`, `-error` and `-empty`. */
  testid: string
}>()
const emit = defineEmits<{ 'open-url': [url: string]; 'open-canvas': [name: string] }>()

const { fontSizePx, lineHeight } = useCanvasSettings()
const readerStyle = computed(() => ({
  '--hv-font-size': `${fontSizePx.value}px`,
  '--hv-line-height': String(lineHeight.value),
}))

const canvasNames = computed(() => new Set(props.metas.map((meta) => meta.name)))

function follow(href: string): void {
  const target = canvasLinkTarget(href, canvasNames.value)
  if (target?.kind === 'url') emit('open-url', target.url)
  else if (target?.kind === 'canvas') emit('open-canvas', target.name)
}

// Links must open in the user's real browser rather than navigate the webview
// away from the app — DetailPane's interception, for the same reason.
function onBodyClick(event: MouseEvent): void {
  const anchor = (event.target as HTMLElement).closest('a')
  if (!anchor) return
  event.preventDefault()
  follow(anchor.getAttribute('href') ?? '')
}
</script>

<template>
  <div class="canvas-reader" :style="readerStyle">
    <AgentCanvasFrontmatter
      v-if="canvas && canvas.createdAt > 0"
      :frontmatter="canvas.frontmatter ?? {}"
      :created-at="canvas.createdAt"
      :updated-at="canvas.updatedAt"
      :testid="testid"
    />
    <template v-if="canvas && canvas.blocks.length">
      <article
        v-for="block in canvas.blocks"
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
    <InlineError
      v-else-if="error"
      :testid="`${testid}-error`"
      variant="line"
      class="leading-relaxed"
      :message="error"
    />
    <EmptyState
      v-else-if="!loading"
      variant="inline"
      message="The agent hasn't put anything here yet."
      :data-testid="`${testid}-empty`"
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
