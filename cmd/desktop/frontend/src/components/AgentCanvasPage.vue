<script setup lang="ts">
// Agents own block and front matter edits; users can only delete a complete
// canvas after confirmation.
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import IconCode from '~icons/lucide/code'
import IconGlobe from '~icons/lucide/globe'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconX from '~icons/lucide/x'
import AgentCanvasActions from './AgentCanvasActions.vue'
import AgentCanvasBrowse from './AgentCanvasBrowse.vue'
import AgentCanvasReader from './AgentCanvasReader.vue'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import BaseButton from './ui/BaseButton.vue'
import EmptyState from './ui/EmptyState.vue'
import IconButton from './ui/IconButton.vue'
import PanelResizeHandle from './ui/PanelResizeHandle.vue'
import ViewHeader from './ui/ViewHeader.vue'
import { useAgentCanvas } from '../composables/useAgentCanvas'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useWailsEvent } from '../composables/useWailsEvent'
import { relativeAge } from '../lib/age'
import {
  globalCanvasOwner,
  globalCanvasOwnerLabel,
  isRepositoryCanvasOwner,
  type CanvasScope,
} from '../lib/agentCanvas'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'
import { useCanvasSettings } from '../stores/useCanvasSettings'

const props = defineProps<{ scope: CanvasScope }>()
const emit = defineEmits<{ close: []; 'open-url': [url: string]; 'update:scope': [scope: CanvasScope]; setup: [] }>()

const { checking, available, reason, client, workspaces, workspacesLoaded, reloadWorkspaces } = useAgentWorkspaces()
const { canvas, metas, shown, loading, error, show, wake } = useAgentCanvas(client)
const { pageWidthClass } = useCanvasSettings()

// The repositories that hold a canvas. A failed read keeps the last list: the
// picker is a way to move, and the canvas on screen does not depend on it.
const repositories = shallowRef<string[]>([])
async function reloadRepositories(): Promise<void> {
  if (!client.value) return
  try {
    repositories.value = await client.value.canvasRepositories()
  } catch {
    // Kept as it was.
  }
}
watch(client, () => void reloadRepositories(), { immediate: true })

// Opened with no owner in scope, outside Chats and Code on a fresh launch, the
// view lands on the first one rather than on nothing.
const workspace = computed(() => props.scope.workspace || workspaces.value[0]?.dir || repositories.value[0] || '')

watch(
  () => [workspace.value, props.scope.name, props.scope.session] as const,
  ([dir, name, session]) => {
    show(dir, name, session)
  },
  { immediate: true },
)

onMounted(() => {
  void reloadWorkspaces()
})

useWailsEvent('canvas:updated', () => {
  wake()
  void reloadRepositories()
})

const workspaceOptions = computed<AppSelectOption[]>(() => {
  const options = [
    ...workspaces.value.map((ws) => ({ value: ws.dir, label: ws.name || ws.dir, icon: IconMessagesSquare })),
    ...repositories.value.map((key) => ({ value: key, label: key, icon: IconCode })),
    { value: globalCanvasOwner, label: globalCanvasOwnerLabel, icon: IconGlobe },
  ]
  // AppSelect draws an unmatched value as blank, and the scope can name an
  // owner neither listing holds yet: a repository with no canvas so far.
  if (workspace.value && !options.some((option) => option.value === workspace.value)) {
    options.push({
      value: workspace.value,
      label: workspace.value,
      icon: isRepositoryCanvasOwner(workspace.value) ? IconCode : IconMessagesSquare,
    })
  }
  return options
})

function selectWorkspace(dir: string): void {
  if (dir !== workspace.value) emit('update:scope', { workspace: dir, name: null, session: null })
}

function openCanvas(name: string): void {
  emit('update:scope', { ...props.scope, workspace: workspace.value, name })
}

function canvasDeleted(name: string): void {
  if (props.scope.name === name) emit('update:scope', { ...props.scope, workspace: workspace.value, name: null })
  wake()
}

const title = computed(() => canvas.value?.title || shown.value || 'Canvases')

// The pane's reason: Hive wires a workspace's agent, never a Code session's.
const offerSetup = computed(
  () => isRepositoryCanvasOwner(workspace.value) && !metas.value.length && !loading.value && !error.value,
)

const reader = ref<HTMLElement | null>(null)
watch(shown, () => {
  if (reader.value) reader.value.scrollTop = 0
})

const {
  size: sidebarWidth,
  startResize: startSidebarResize,
  step: stepSidebar,
} = useResizablePanel({
  storageKey: 'hive.panel.canvas.sidebar',
  defaultSize: 280,
  min: 200,
  max: 480,
  edge: 'right',
})

useEscapeToClose(() => emit('close'))
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="canvas-page">
    <ViewHeader>
      <template #title>
        <span class="min-w-0 truncate text-body font-semibold text-text" data-testid="canvas-page-title">{{
          title
        }}</span>
        <span v-if="canvas && shown" class="shrink-0 font-mono text-caption text-text-4"
          >{{ shown }} · {{ relativeAge(canvas.updatedAt) }}</span
        >
        <div class="flex-1" />
        <AgentCanvasActions
          v-if="shown"
          :workspace="workspace"
          :name="shown"
          :client="client"
          size="lg"
          testid="canvas-page"
          @deleted="canvasDeleted"
        />
        <IconButton label="Close" :icon="IconX" size="lg" data-testid="canvas-page-close" @click="emit('close')" />
      </template>
    </ViewHeader>

    <!-- The Chats area's own answer when this build cannot run it: there is no
         client to read a canvas with. -->
    <EmptyState v-if="!checking && !available" class="m-auto max-w-md" data-testid="canvas-page-unavailable">
      {{ reason || 'Canvases are unavailable.' }}
    </EmptyState>
    <EmptyState
      v-else-if="workspacesLoaded && !workspace"
      class="m-auto"
      message="No workspaces or repositories with canvases yet."
      data-testid="canvas-page-no-workspaces"
    />
    <div v-else class="flex min-h-0 flex-1">
      <aside
        class="relative flex shrink-0 flex-col border-r border-border bg-sidebar"
        :style="{ width: sidebarWidth + 'px' }"
        data-testid="canvas-page-sidebar"
      >
        <div class="shrink-0 border-b border-border p-2">
          <AppSelect
            :model-value="workspace"
            :options="workspaceOptions"
            size="sm"
            aria-label="Workspace or repository"
            testid="canvas-page-workspace"
            @update:model-value="selectWorkspace"
          />
        </div>
        <AgentCanvasBrowse :metas="metas" :shown="shown" testid="canvas-page" @pick="openCanvas" />
        <PanelResizeHandle edge="right" name="canvas-sidebar" :start="startSidebarResize" :step="stepSidebar" />
      </aside>

      <div
        ref="reader"
        class="hive-scroll min-h-0 min-w-0 flex-1 overflow-y-auto bg-app"
        data-testid="canvas-page-reader"
      >
        <div class="mx-auto w-full px-10 py-8" :class="pageWidthClass" data-testid="canvas-page-measure">
          <AgentCanvasReader
            :canvas="canvas"
            :metas="metas"
            :loading="loading"
            :error="error"
            testid="canvas-page"
            @open-url="emit('open-url', $event)"
            @open-canvas="openCanvas"
          />
          <div v-if="offerSetup" class="mt-3 flex flex-col items-start gap-2" data-testid="canvas-page-setup">
            <p class="text-xs leading-relaxed text-text-3">
              An agent in a Code session of this repository can write here once its own MCP configuration lists the Hive
              Canvas server.
            </p>
            <BaseButton variant="secondary" size="xs" data-testid="canvas-page-setup-open" @click="emit('setup')">
              Set up an agent
            </BaseButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
