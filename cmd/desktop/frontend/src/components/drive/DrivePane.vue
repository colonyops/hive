<script setup lang="ts">
// The drive beside a chat or a Code session: the space's documents, opened on
// the one the session wrote last. Agents write the content; the user reads,
// copies, exports, and deletes.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconMaximize2 from '~icons/lucide/maximize-2'
import IconX from '~icons/lucide/x'
import DriveBrowser from './DriveBrowser.vue'
import DriveDocumentActions from './DriveDocumentActions.vue'
import DriveReader from './DriveReader.vue'
import DriveReviewPanel from './DriveReviewPanel.vue'
import BaseButton from '../ui/BaseButton.vue'
import IconButton from '../ui/IconButton.vue'
import PanelResizeHandle from '../ui/PanelResizeHandle.vue'
import { useDriveDocument } from '../../composables/useDriveDocument'
import { useResizablePanel } from '../../composables/useResizablePanel'
import { useDriveActivity } from '../../stores/useDriveActivity'
import { entryLabel, movedPath, newestDocument, type DriveAuthor } from '../../lib/drive'
import type { AgentTargetRef } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{
  /** The open chat or Code session, whose newest write is the default pick. */
  session: DriveAuthor
  /** The space the tree shows: the chat's workspace, or the session's repository. */
  space: string
  /** The route-pinned document path; null lets the default win. */
  path: string | null
  /** Offers the way to connect an agent while the space is empty, where Hive does not wire the agent itself. */
  agentSetup?: boolean
  /** The surface's own agent, which Send to Agent offers first. */
  agent?: AgentTargetRef | null
}>()
const emit = defineEmits<{
  close: []
  'open-url': [url: string]
  pick: [path: string | null]
  'open-page': [path: string | null]
  setup: []
}>()

const { document, tree, treeError, paths, shown, loading, error, show } = useDriveDocument({
  vanished: (path) => documentDeleted(path),
})
const activity = useDriveActivity()

watch(
  () => props.session,
  (session, before) => {
    if (before !== undefined) activity.unregisterVisible(before)
    activity.registerVisible(session)
  },
  { immediate: true },
)
onBeforeUnmount(() => activity.unregisterVisible(props.session))

watch(
  () => [props.space, props.path, props.session] as const,
  ([space, path, session]) => {
    show(space, path, session)
  },
  { immediate: true },
)

// The header title opens the tree over the content (the Grafana-assistant
// pattern) rather than a dropdown.
const browsing = ref(false)

const headerTitle = computed(() => (document.value ? entryLabel(document.value.entry) : shown.value) || 'Drive')

// A pick pins the path in the route; the prop watcher above brings it back.
function pick(path: string): void {
  browsing.value = false
  emit('pick', path)
}

function documentDeleted(path: string): void {
  if (path !== shown.value && path !== props.path) return
  const next = newestDocument(tree.value)
  emit('pick', next && next.path !== path ? next.path : null)
}

function documentMoved(from: string, to: string): void {
  const current = shown.value
  if (current && movedPath(current, from, to) !== current) emit('pick', movedPath(current, from, to))
}

// A link to another document lands at its top, not at the offset the last
// one was scrolled to.
const reader = ref<HTMLElement | null>(null)
watch(shown, () => {
  if (reader.value) reader.value.scrollTop = 0
})

const empty = computed(() => !!tree.value && !tree.value.children?.length)

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
    data-testid="drive-pane"
  >
    <PanelResizeHandle edge="left" name="drive-pane" :start="startPaneResize" :step="stepPane" />

    <div class="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1">
      <button
        type="button"
        class="flex h-6 min-w-0 cursor-pointer items-center gap-1 rounded-lg px-1.5 hover:bg-chip"
        :aria-expanded="browsing"
        aria-label="Browse documents"
        data-testid="drive-pane-title"
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
      <DriveDocumentActions
        v-if="document"
        :space="space"
        :entry="document.entry"
        testid="drive-pane"
        @deleted="documentDeleted"
        @moved="documentMoved"
      />
      <IconButton
        label="Open full page"
        :icon="IconMaximize2"
        data-testid="drive-pane-expand"
        @click="emit('open-page', shown)"
      />
      <IconButton label="Close drive" :icon="IconX" data-testid="drive-pane-close" @click="emit('close')" />
    </div>

    <DriveReviewPanel
      v-if="!browsing && document?.entry.kind === 'markdown'"
      :space="document.space.key"
      :path="document.entry.path"
      :agent="agent"
      testid="drive-pane"
    />

    <DriveBrowser
      v-if="browsing"
      :space="space"
      :tree="tree"
      :selected="shown"
      :error="treeError"
      compact
      autofocus
      testid="drive-pane"
      @pick="pick"
      @moved="documentMoved"
      @deleted="documentDeleted"
      @escape="browsing = false"
    />

    <!-- Positioned so an html document fills the reader, not the pane. -->
    <div v-else class="relative flex min-h-0 flex-1 flex-col">
      <div ref="reader" class="hive-scroll flex min-h-0 flex-1 flex-col overflow-y-auto px-4 py-3">
        <DriveReader
          :document="document"
          :paths="paths"
          :loading="loading"
          :error="error ?? treeError"
          testid="drive-pane"
          @open-url="emit('open-url', $event)"
          @open="pick"
        />
        <div
          v-if="agentSetup && empty && !loading"
          class="mt-3 flex flex-col items-start gap-2"
          data-testid="drive-pane-setup"
        >
          <p class="text-xs leading-relaxed text-text-3">
            An agent in this session can write here once its own MCP configuration lists the Hive Drive server.
          </p>
          <BaseButton variant="secondary" size="xs" data-testid="drive-pane-setup-open" @click="emit('setup')">
            Set up an agent
          </BaseButton>
        </div>
      </div>
    </div>
  </aside>
</template>
