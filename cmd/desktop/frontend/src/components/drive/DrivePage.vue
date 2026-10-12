<script setup lang="ts">
// Every space's documents at full page: the space picker in the header over
// the sidebar's tree, the reader beside it, and a search that spans every
// space.
import { computed, ref, watch } from 'vue'
import IconCode from '~icons/lucide/code'
import IconGlobe from '~icons/lucide/globe'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconX from '~icons/lucide/x'
import DriveBrowser from './DriveBrowser.vue'
import DriveDocumentActions from './DriveDocumentActions.vue'
import DriveReader from './DriveReader.vue'
import AppSelect, { type AppSelectOption } from '../ui/AppSelect.vue'
import BaseButton from '../ui/BaseButton.vue'
import EmptyState from '../ui/EmptyState.vue'
import IconButton from '../ui/IconButton.vue'
import InlineError from '../ui/InlineError.vue'
import PanelResizeHandle from '../ui/PanelResizeHandle.vue'
import ViewHeader from '../ui/ViewHeader.vue'
import { useDriveDocument } from '../../composables/useDriveDocument'
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import { useResizablePanel } from '../../composables/useResizablePanel'
import { relativeAge } from '../../lib/age'
import { entryLabel, entryTime, movedPath, spaceGroup, spaceGroups, type DriveScope } from '../../lib/drive'
import { useCanvasSettings } from '../../stores/useCanvasSettings'
import { useDrive } from '../../stores/useDrive'

const props = defineProps<{ scope: DriveScope }>()
const emit = defineEmits<{ close: []; 'open-url': [url: string]; 'update:scope': [scope: DriveScope]; setup: [] }>()

const { spaces, spacesLoaded, error: spacesError } = useDrive()
const { document, tree, treeError, paths, shown, loading, error, show } = useDriveDocument()
const { pageWidthClass } = useCanvasSettings()

// Opened with no space in scope, outside Chats and Code on a fresh launch, the
// view lands on the first one rather than on nothing.
const space = computed(() => props.scope.space || spaces.value[0]?.key || '')

watch(
  () => [space.value, props.scope.path, props.scope.session] as const,
  ([key, path, session]) => {
    show(key, path, session)
  },
  { immediate: true },
)

const groupIcons = { Repositories: IconCode, Workspaces: IconMessagesSquare, Shared: IconGlobe }

const spaceOptions = computed<AppSelectOption[]>(() => {
  const listed = [...spaces.value]
  // A scope can name a space the listing does not hold yet: a workspace no
  // agent has written to.
  if (space.value && !listed.some((known) => known.key === space.value)) {
    listed.push({ kind: '', key: space.value, name: space.value.replace(/^workspaces\//, '') })
  }
  return spaceGroups.flatMap((group) =>
    listed
      .filter((known) => spaceGroup(known.key) === group)
      .map((known) => ({ value: known.key, label: known.name || known.key, icon: groupIcons[group], group })),
  )
})

function selectSpace(key: string): void {
  if (key !== space.value) emit('update:scope', { space: key, path: null, session: null })
}

function openDocument(path: string): void {
  emit('update:scope', { ...props.scope, space: space.value, path })
}

function openHit(key: string, path: string): void {
  emit('update:scope', { space: key, path, session: null })
}

function documentDeleted(path: string): void {
  if (props.scope.path === path) emit('update:scope', { ...props.scope, space: space.value, path: null })
}

// The scope may name no path while the default is on screen, so the shown
// document is what follows a move.
function documentMoved(from: string, to: string): void {
  const current = shown.value
  if (current && movedPath(current, from, to) !== current) {
    emit('update:scope', { ...props.scope, space: space.value, path: movedPath(current, from, to) })
  }
}

const title = computed(() => (document.value ? entryLabel(document.value.entry) : shown.value) || 'Drive')
// The title already names the document, so only its folder sits beside it.
const folder = computed(() => {
  const path = document.value?.entry.path ?? ''
  const slash = path.lastIndexOf('/')
  return slash < 0 ? '' : path.slice(0, slash)
})
const modified = computed(() => (document.value ? entryTime(document.value.entry.modified) : 0))

// The pane's reason: Hive wires a workspace's agent, never a Code session's.
const offerSetup = computed(
  () =>
    spaceGroup(space.value) === 'Repositories' &&
    !!tree.value &&
    !tree.value.children?.length &&
    !loading.value &&
    !error.value,
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

// The sidebar, with the picker as its heading, is there whenever there is a
// space to browse; a listing error or an empty drive leaves the reader
// column alone to say so.
const browsable = computed(() => !(spacesError.value && !spaces.value.length) && !(spacesLoaded.value && !space.value))

useEscapeToClose(() => emit('close'))
</script>

<template>
  <div class="flex h-full min-h-0 flex-1" data-testid="drive-page">
    <aside
      v-if="browsable"
      class="relative flex shrink-0 flex-col border-r border-border bg-sidebar"
      :style="{ width: sidebarWidth + 'px' }"
      data-testid="drive-page-sidebar"
    >
      <!-- The picker is the tree's heading: a flat row in the title strip's
           own colour and height, so the strip runs on across the sidebar and
           the picker shows no chrome of its own. -->
      <div class="flex h-11 shrink-0 items-center border-b border-row bg-canvas-toolbar px-1.5">
        <AppSelect
          class="min-w-0 flex-1"
          :model-value="space"
          :options="spaceOptions"
          variant="flat"
          size="sm"
          searchable
          search-placeholder="Find a space"
          aria-label="Space"
          testid="drive-page-space"
          @update:model-value="selectSpace"
        />
      </div>
      <DriveBrowser
        :space="space"
        :tree="tree"
        :selected="shown"
        :error="treeError"
        search-every-space
        testid="drive-page"
        @pick="openDocument"
        @open-hit="openHit"
        @moved="documentMoved"
        @deleted="documentDeleted"
      />
      <PanelResizeHandle edge="right" name="canvas-sidebar" :start="startSidebarResize" :step="stepSidebar" />
    </aside>

    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      <ViewHeader>
        <template #title>
          <span
            class="min-w-0 truncate text-body font-semibold text-text"
            :title="document?.entry.path"
            data-testid="drive-page-title"
            >{{ title }}</span
          >
          <span
            v-if="document && (folder || modified)"
            class="shrink-0 font-mono text-caption text-text-4"
            data-testid="drive-page-folder"
            >{{ folder }}<template v-if="folder && modified"> · </template
            ><template v-if="modified">{{ relativeAge(modified) }}</template></span
          >
          <div class="flex-1" />
          <DriveDocumentActions
            v-if="document"
            :space="space"
            :entry="document.entry"
            size="lg"
            testid="drive-page"
            @deleted="documentDeleted"
            @moved="documentMoved"
          />
          <IconButton label="Close" :icon="IconX" size="lg" data-testid="drive-page-close" @click="emit('close')" />
        </template>
      </ViewHeader>

      <InlineError v-if="spacesError && !spaces.length" class="m-4" :message="spacesError" testid="drive-page-error" />
      <EmptyState
        v-else-if="spacesLoaded && !space"
        class="m-auto"
        message="No spaces yet. A repository, a workspace, or the shared space appears here once a document is written to it."
        data-testid="drive-page-no-spaces"
      />
      <div
        v-else
        ref="reader"
        class="hive-scroll flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto bg-app"
        data-testid="drive-page-reader"
      >
        <div
          class="mx-auto flex w-full flex-1 flex-col px-10 py-8"
          :class="pageWidthClass"
          data-testid="drive-page-measure"
        >
          <DriveReader
            :document="document"
            :paths="paths"
            :loading="loading"
            :error="error"
            testid="drive-page"
            @open-url="emit('open-url', $event)"
            @open="openDocument"
          />
          <div v-if="offerSetup" class="mt-3 flex flex-col items-start gap-2" data-testid="drive-page-setup">
            <p class="text-xs leading-relaxed text-text-3">
              An agent in a Code session of this repository can write here once its own MCP configuration lists the Hive
              Drive server.
            </p>
            <BaseButton variant="secondary" size="xs" data-testid="drive-page-setup-open" @click="emit('setup')">
              Set up an agent
            </BaseButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
