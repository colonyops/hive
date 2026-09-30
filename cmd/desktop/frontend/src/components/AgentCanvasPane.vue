<script setup lang="ts">
// A workspace's canvases: agent-written markdown, html and link blocks,
// read-only in the webview — writes arrive only through the hive-canvas MCP
// tools, so this pane re-reads on canvas:updated rather than ever mutating
// (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry).
import { computed, nextTick, ref, toRef, watch } from 'vue'
import { Dialogs } from '@wailsio/runtime'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconCopy from '~icons/lucide/copy'
import IconDownload from '~icons/lucide/download'
import IconFileText from '~icons/lucide/file-text'
import IconSearch from '~icons/lucide/search'
import IconX from '~icons/lucide/x'
import PanelResizeHandle from './PanelResizeHandle.vue'
import { useAgentCanvas } from '../composables/useAgentCanvas'
import { useCanvasTypography } from '../composables/useCanvasTypography'
import { useClipboard } from '../composables/useClipboard'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useWailsEvent } from '../composables/useWailsEvent'
import { relativeAge } from '../lib/age'
import { renderGithubMarkdown } from '../lib/githubMarkdown'
import type { AgentWorkspacesClient, CanvasBlock, WorkspaceCanvasMeta } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  /** The open chat, whose most recent canvas is the default pick. */
  session: number
  /** The open chat's workspace, whose canvases fill the picker. */
  workspace: string
  /** The route-pinned canvas name; null lets the default win. */
  name: string | null
  client: AgentWorkspacesClient | null
}>()
const emit = defineEmits<{ close: []; 'open-url': [url: string]; pick: [name: string] }>()

const { canvas, metas, shown, loading, error, show, wake } =
  useAgentCanvas(toRef(props, 'client'))

watch(() => [props.workspace, props.name, props.session] as const, ([dir, name, session]) => {
  show(dir, name, session)
}, { immediate: true })

// The header title opens an in-pane browse list over the content (the
// Grafana-assistant pattern) rather than a dropdown: rows are the
// workspace's canvases, the current one marked, each aged on the right.
const browsing = ref(false)

const headerTitle = computed(() => canvas.value?.title || shown.value || 'Canvas')

const { fontSizePx, lineHeight } = useCanvasTypography()
const readerStyle = computed(() => ({
  '--hv-font-size': `${fontSizePx.value}px`,
  '--hv-line-height': String(lineHeight.value),
}))

const search = ref('')
const searchInput = ref<HTMLInputElement | null>(null)
watch(browsing, (open) => {
  if (!open) return
  search.value = ''
  void nextTick(() => searchInput.value?.focus())
})

// The Code sidebar filter's escape ladder: a first Esc clears the text, a
// second leaves the browse view.
function escapeSearch(): void {
  if (search.value) search.value = ''
  else browsing.value = false
}

// Copy is fetch-first (usePrompts' shape): the Go side renders the markdown
// so copy and save can never disagree, and a failure before SetText is still
// a failed copy as far as the user is concerned.
const { copy, setStatus: setCopyStatus, status: copyStatus } = useClipboard()
async function copyCanvas(): Promise<void> {
  const name = shown.value
  if (!name || !props.client) return
  try {
    await copy(await props.client.canvasMarkdown(props.workspace, name))
  } catch {
    setCopyStatus('error')
  }
}

// Status mechanics only — the same auto-resetting affordance, driving the
// save button instead of a clipboard.
const { setStatus: setSaveStatus, status: saveStatus } = useClipboard()
async function downloadCanvas(): Promise<void> {
  const name = shown.value
  if (!name || !props.client) return
  try {
    const path = await Dialogs.SaveFile({ Filename: `${name}.md`, Title: 'Export canvas' })
    if (!path) return
    await props.client.exportCanvas(props.workspace, name, path)
    setSaveStatus('success')
  } catch {
    setSaveStatus('error')
  }
}

const filteredMetas = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return metas.value
  return metas.value.filter((meta) =>
    meta.name.toLowerCase().includes(query) || meta.title.toLowerCase().includes(query))
})

const DAY_MS = 24 * 60 * 60 * 1000

function activityGroup(updatedAt: number, now: number): string {
  const age = now - updatedAt
  if (age < DAY_MS) return 'Today'
  if (age < 7 * DAY_MS) return 'Last week'
  if (age < 30 * DAY_MS) return 'Last 30 days'
  return 'Older'
}

// The listing arrives most-recently-updated first, so one sequential pass
// yields the groups already in display order.
const groupedMetas = computed(() => {
  const now = Date.now()
  const groups: Array<{ label: string; metas: WorkspaceCanvasMeta[] }> = []
  for (const meta of filteredMetas.value) {
    const label = activityGroup(meta.updatedAt, now)
    const last = groups[groups.length - 1]
    if (last?.label === label) last.metas.push(meta)
    else groups.push({ label, metas: [meta] })
  }
  return groups
})

// A pick pins the name in the route; the prop watcher above brings it back.
function pick(name: string): void {
  browsing.value = false
  emit('pick', name)
}

// Every write re-reads both the shown canvas and the browse listing: the
// signal's payload can be coalesced away, and both reads are cheap.
useWailsEvent('canvas:updated', () => wake())

// Both body kinds are safe for v-html, by two different routes. Markdown goes
// through renderGithubMarkdown, which escapes raw HTML and drops unsafe link
// schemes. An html block already arrived sanitized: canvas.SanitizeHTML runs on
// the Go read path, so there is exactly one policy and the pane holds none of
// it (ADR canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary).
function renderBody(block: CanvasBlock): string {
  return block.kind === 'html' ? block.body : renderGithubMarkdown(block.body)
}

// Links must open in the user's real browser rather than navigate the webview
// away from the app — DetailPane's interception, for the same reason.
function onBodyClick(event: MouseEvent): void {
  const anchor = (event.target as HTMLElement).closest('a')
  if (!anchor) return
  event.preventDefault()
  const href = anchor.getAttribute('href') ?? ''
  if (/^(https?:|mailto:)/i.test(href)) emit('open-url', href)
}

function openLinkBlock(block: CanvasBlock): void {
  if (/^(https?:|mailto:)/i.test(block.url)) emit('open-url', block.url)
}

const { size: paneWidth, startResize: startPaneResize, step: stepPane } = useResizablePanel({
  storageKey: 'hive.panel.agents.canvas',
  defaultSize: 380,
  min: 300,
  max: 900,
  edge: 'left',
})
</script>

<template>
  <aside
    class="relative flex shrink-0 flex-col border-l border-border bg-pane"
    :style="{ width: paneWidth + 'px' }"
    data-testid="agent-canvas-pane"
  >
    <PanelResizeHandle edge="left" name="agents-canvas" :start="startPaneResize" :step="stepPane" />

    <div class="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1">
      <button
        type="button"
        class="flex h-6 min-w-0 cursor-pointer items-center gap-1 rounded-[7px] px-1.5 hover:bg-chip"
        :aria-expanded="browsing"
        aria-label="Browse canvases"
        data-testid="agent-canvas-title"
        @click="browsing = !browsing"
      >
        <span class="min-w-0 truncate text-[12px] font-semibold text-text">{{ headerTitle }}</span>
        <IconChevronDown
          class="size-3.5 shrink-0 text-text-3 transition-transform"
          :class="browsing ? 'rotate-180' : ''"
          aria-hidden="true"
        />
      </button>
      <div class="min-w-0 flex-1" />
      <button
        v-if="shown"
        type="button"
        class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] hover:bg-chip hover:text-text"
        :class="copyStatus === 'error' ? 'text-severity-error' : 'text-text-3'"
        :title="copyStatus === 'success' ? 'Copied' : 'Copy as Markdown'"
        :aria-label="copyStatus === 'success' ? 'Copied' : 'Copy as Markdown'"
        data-testid="agent-canvas-copy"
        @click="copyCanvas"
      ><component :is="copyStatus === 'success' ? IconCheck : IconCopy" class="size-3.5" /></button>
      <button
        v-if="shown"
        type="button"
        class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] hover:bg-chip hover:text-text"
        :class="saveStatus === 'error' ? 'text-severity-error' : 'text-text-3'"
        :title="saveStatus === 'success' ? 'Saved' : 'Save as Markdown…'"
        :aria-label="saveStatus === 'success' ? 'Saved' : 'Save as Markdown…'"
        data-testid="agent-canvas-download"
        @click="downloadCanvas"
      ><component :is="saveStatus === 'success' ? IconCheck : IconDownload" class="size-3.5" /></button>
      <button
        type="button"
        class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text"
        title="Close canvas"
        aria-label="Close canvas"
        data-testid="agent-canvas-close"
        @click="emit('close')"
      ><IconX class="size-3.5" /></button>
    </div>

    <div v-if="browsing" class="flex min-h-0 flex-1 flex-col" data-testid="agent-canvas-browse">
      <!-- Flush in the bar, the Code sidebar's filter shape: a boxed field in
           a pane this narrow reads as chrome. -->
      <div class="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
        <IconSearch class="size-3 shrink-0" :class="search ? 'text-text-3' : 'text-text-4'" />
        <input
          ref="searchInput"
          v-model="search"
          type="text"
          placeholder="Filter…"
          aria-label="Filter canvases"
          class="min-w-0 flex-1 bg-transparent text-[12.5px] text-text outline-none placeholder:text-text-4"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          data-testid="agent-canvas-search"
          @keydown.esc.prevent="escapeSearch"
        >
      </div>
      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto pb-2 pt-1">
        <template v-for="group in groupedMetas" :key="group.label">
          <p class="px-3 pb-1 pt-2.5 text-[10.5px] font-medium uppercase tracking-wide text-text-4">{{ group.label }}</p>
          <div class="divide-y divide-border">
            <button
              v-for="meta in group.metas"
              :key="meta.name"
              type="button"
              class="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left hover:bg-chip"
              :aria-current="meta.name === shown ? 'true' : undefined"
              :data-testid="'agent-canvas-browse-' + meta.name"
              @click="pick(meta.name)"
            >
              <IconFileText class="size-3.5 shrink-0" :class="meta.name === shown ? 'text-accent' : 'text-text-4'" aria-hidden="true" />
              <span class="min-w-0 flex-1 truncate text-[12.5px]" :class="meta.name === shown ? 'text-text' : 'text-text-2'">{{ meta.title || meta.name }}</span>
              <span class="shrink-0 font-mono text-[10.5px] text-text-4">{{ relativeAge(meta.updatedAt) }}</span>
            </button>
          </div>
        </template>
        <p v-if="!filteredMetas.length" class="px-3 py-2 text-xs leading-relaxed text-text-4">
          {{ metas.length ? 'No canvases match.' : 'No canvases yet.' }}
        </p>
      </div>
    </div>

    <div v-else class="hive-scroll canvas-reader min-h-0 flex-1 overflow-y-auto px-4 py-3" :style="readerStyle">
      <template v-if="canvas && canvas.blocks.length">
        <article
          v-for="block in canvas.blocks"
          :key="block.id"
          class="canvas-block"
          :data-testid="'agent-canvas-block-' + block.id"
        >
          <template v-if="block.kind === 'markdown' || block.kind === 'html'">
            <h2 v-if="block.title" class="canvas-block-title mb-2 font-semibold text-text">{{ block.title }}</h2>
            <div
              class="canvas-reading-body text-text-2"
              :class="block.kind === 'html' ? 'hv-html' : 'markdown-body'"
              :style="readerStyle"
              @click="onBodyClick"
              v-html="renderBody(block)"
            />
          </template>
          <button
            v-else-if="block.kind === 'link'"
            type="button"
            class="canvas-link"
            :title="block.url"
            @click="openLinkBlock(block)"
          >
            <span class="canvas-link-title truncate text-accent underline underline-offset-2">{{ block.title }}</span>
            <span class="canvas-link-url truncate font-mono text-text-4">{{ block.url }}</span>
          </button>
        </article>
      </template>
      <p v-else-if="error" class="text-xs leading-relaxed text-severity-error" data-testid="agent-canvas-error">{{ error }}</p>
      <p v-else-if="!loading" class="text-xs leading-relaxed text-text-4" data-testid="agent-canvas-empty">
        The agent hasn't put anything here yet.
      </p>
    </div>
  </aside>
</template>

<style scoped>
.canvas-reader { font-size: var(--hv-font-size); line-height: var(--hv-line-height); }
.canvas-block { padding: 12px 0; }
.canvas-block + .canvas-block { border-top: 1px solid var(--color-border); }
.canvas-block:first-child { padding-top: 0; }
.canvas-block-title, .canvas-link-title { font-size: 1em; line-height: var(--hv-line-height); }
.canvas-reading-body { font-size: var(--hv-font-size); line-height: var(--hv-line-height); }
.canvas-reading-body.markdown-body :deep(h1),
.canvas-reading-body.markdown-body :deep(h2),
.canvas-reading-body.markdown-body :deep(h3),
.canvas-reading-body.markdown-body :deep(h4),
.canvas-reading-body.markdown-body :deep(h5),
.canvas-reading-body.markdown-body :deep(h6) { line-height: calc(var(--hv-line-height) * 0.79); }
.canvas-reading-body.markdown-body :deep(h1) { font-size: 1.407em; }
.canvas-reading-body.markdown-body :deep(h2) { font-size: 1.222em; }
.canvas-reading-body.markdown-body :deep(h3) { font-size: 1.111em; }
.canvas-reading-body.markdown-body :deep(h4),
.canvas-reading-body.markdown-body :deep(h5),
.canvas-reading-body.markdown-body :deep(h6) { font-size: 1.037em; }
.canvas-reading-body.markdown-body :deep(pre) { line-height: calc(var(--hv-line-height) * 0.91); }
.canvas-link { display: flex; width: 100%; min-width: 0; cursor: pointer; flex-direction: column; align-items: flex-start; gap: 2px; text-align: left; }
.canvas-link:hover span:first-child { text-decoration-thickness: 2px; }
.canvas-link-url { font-size: 0.778em; line-height: var(--hv-line-height); }
</style>
