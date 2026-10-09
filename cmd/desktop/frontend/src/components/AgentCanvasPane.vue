<script setup lang="ts">
// A workspace's canvases: agent-written markdown, Mermaid, html and link blocks.
// Block and front matter writes arrive through the hive-canvas MCP tools. The
// user may delete the canvas after confirmation.
import IconButton from './ui/IconButton.vue'
import { computed, ref, toRef, watch } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconMaximize2 from '~icons/lucide/maximize-2'
import IconX from '~icons/lucide/x'
import AgentCanvasActions from './AgentCanvasActions.vue'
import AgentCanvasBrowse from './AgentCanvasBrowse.vue'
import AgentCanvasReader from './AgentCanvasReader.vue'
import BaseButton from './ui/BaseButton.vue'
import PanelResizeHandle from './ui/PanelResizeHandle.vue'
import { useAgentCanvas } from '../composables/useAgentCanvas'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useWailsEvent } from '../composables/useWailsEvent'
import type { CanvasAuthor } from '../lib/agentCanvas'
import type { AgentWorkspacesClient } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  /** The open chat or Code session, whose most recent canvas is the default pick. */
  session: CanvasAuthor
  /** The owner whose canvases fill the picker: the chat's workspace, or the session's repository. */
  workspace: string
  /** The route-pinned canvas name; null lets the default win. */
  name: string | null
  client: AgentWorkspacesClient | null
  /** Offers the way to connect an agent while there is no canvas, where Hive does not wire the agent itself. */
  agentSetup?: boolean
}>()
const emit = defineEmits<{
  close: []
  'open-url': [url: string]
  pick: [name: string | null]
  'open-page': [name: string | null]
  setup: []
}>()

const { canvas, metas, shown, loading, error, show, wake } = useAgentCanvas(toRef(props, 'client'))

watch(
  () => [props.workspace, props.name, props.session] as const,
  ([dir, name, session]) => {
    show(dir, name, session)
  },
  { immediate: true },
)

// The header title opens an in-pane browse list over the content (the
// Grafana-assistant pattern) rather than a dropdown: rows are the
// workspace's canvases, the current one marked, each aged on the right.
const browsing = ref(false)

const headerTitle = computed(() => canvas.value?.title || shown.value || 'Canvas')

// A pick pins the name in the route; the prop watcher above brings it back.
function pick(name: string): void {
  browsing.value = false
  emit('pick', name)
}

function canvasDeleted(name: string): void {
  emit('pick', metas.value.find((meta) => meta.name !== name)?.name ?? null)
  wake()
}

// A link to another canvas lands at its top, not at the offset the last one
// was scrolled to.
const reader = ref<HTMLElement | null>(null)
watch(shown, () => {
  if (reader.value) reader.value.scrollTop = 0
})

// Every write re-reads both the shown canvas and the browse listing: the
// signal's payload can be coalesced away, and both reads are cheap.
useWailsEvent('canvas:updated', () => wake())

const {
  size: paneWidth,
  startResize: startPaneResize,
  step: stepPane,
} = useResizablePanel({
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
        class="flex h-6 min-w-0 cursor-pointer items-center gap-1 rounded-lg px-1.5 hover:bg-chip"
        :aria-expanded="browsing"
        aria-label="Browse canvases"
        data-testid="agent-canvas-title"
        @click="browsing = !browsing"
      >
        <span class="min-w-0 truncate text-small font-semibold text-text">{{ headerTitle }}</span>
        <IconChevronDown
          class="size-3.5 shrink-0 text-text-3 transition-transform"
          :class="browsing ? 'rotate-180' : ''"
          aria-hidden="true"
        />
      </button>
      <div class="min-w-0 flex-1" />
      <AgentCanvasActions
        v-if="shown"
        :workspace="workspace"
        :name="shown"
        :client="client"
        testid="agent-canvas"
        @deleted="canvasDeleted"
      />
      <IconButton
        label="Open full page"
        :icon="IconMaximize2"
        data-testid="agent-canvas-expand"
        @click="emit('open-page', shown)"
      />
      <IconButton label="Close canvas" :icon="IconX" data-testid="agent-canvas-close" @click="emit('close')" />
    </div>

    <AgentCanvasBrowse
      v-if="browsing"
      :metas="metas"
      :shown="shown"
      autofocus
      testid="agent-canvas"
      @pick="pick"
      @escape="browsing = false"
    />

    <div v-else ref="reader" class="hive-scroll min-h-0 flex-1 overflow-y-auto px-4 py-3">
      <AgentCanvasReader
        :canvas="canvas"
        :metas="metas"
        :loading="loading"
        :error="error"
        testid="agent-canvas"
        @open-url="emit('open-url', $event)"
        @open-canvas="pick"
      />
      <div
        v-if="agentSetup && !metas.length && !loading && !error"
        class="mt-3 flex flex-col items-start gap-2"
        data-testid="agent-canvas-setup"
      >
        <p class="text-xs leading-relaxed text-text-3">
          An agent in this session can write here once its own MCP configuration lists the Hive Canvas server.
        </p>
        <BaseButton variant="secondary" size="xs" data-testid="agent-canvas-setup-open" @click="emit('setup')">
          Set up an agent
        </BaseButton>
      </div>
    </div>
  </aside>
</template>
