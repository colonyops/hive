<script setup lang="ts">
// One canvas rendered block by block. The pane and the full-page view share
// it, so a canvas reads the same in both and only the width differs.
import { computed } from 'vue'
import EmptyState from './ui/EmptyState.vue'
import InlineError from './ui/InlineError.vue'
import { useCanvasSettings } from '../stores/useCanvasSettings'
import { canvasLinkTarget } from '../lib/agentCanvas'
import { renderGithubMarkdown } from '../lib/githubMarkdown'
import type { CanvasBlock, WorkspaceCanvas, WorkspaceCanvasMeta } from '../lib/agentWorkspacesClient'

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

// Both body kinds are safe for v-html, by two different routes. Markdown goes
// through renderGithubMarkdown, which escapes raw HTML and drops unsafe link
// schemes. An html block already arrived sanitized: canvas.SanitizeHTML runs on
// the Go read path, so there is exactly one policy and the reader holds none of
// it (ADR canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary).
function renderBody(block: CanvasBlock): string {
  return block.kind === 'html' ? block.body : renderGithubMarkdown(block.body)
}

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
    <template v-if="canvas && canvas.blocks.length">
      <article
        v-for="block in canvas.blocks"
        :key="block.id"
        class="canvas-block"
        :data-testid="`${testid}-block-${block.id}`"
      >
        <template v-if="block.kind === 'markdown' || block.kind === 'html'">
          <h2 v-if="block.title" class="canvas-block-title mb-2 font-semibold text-text">{{ block.title }}</h2>
          <!-- eslint-disable vue/no-v-html -- markdown goes through renderGithubMarkdown; html blocks arrive sanitized by canvas.SanitizeHTML in Go -->
          <div
            class="canvas-reading-body text-text-2"
            :class="block.kind === 'html' ? 'hv-html' : 'markdown-body'"
            :style="readerStyle"
            @click="onBodyClick"
            v-html="renderBody(block)"
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
          <span class="canvas-link-title truncate text-accent underline underline-offset-2">{{ block.title }}</span>
          <span class="canvas-link-url truncate font-mono text-text-4">{{ block.url }}</span>
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
.canvas-block-title,
.canvas-link-title {
  font-size: 1em;
  line-height: var(--hv-line-height);
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
.canvas-link {
  display: flex;
  width: 100%;
  min-width: 0;
  cursor: pointer;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  text-align: left;
}
.canvas-link:hover span:first-child {
  text-decoration-thickness: 2px;
}
.canvas-link-url {
  font-size: 0.778em;
  line-height: var(--hv-line-height);
}
</style>
