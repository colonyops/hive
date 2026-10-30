<script setup lang="ts">
// The right slot in Code. It owns the aside, its width and the switcher, so
// Chats can keep mounting DrivePane as it is.
import { computed, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import IconList from '~icons/lucide/list'
import IconListTree from '~icons/lucide/list-tree'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import PanelResizeHandle from '@hive/ui/components/ui/PanelResizeHandle.vue'
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import { useResizablePanel } from '@hive/ui/composables/useResizablePanel'
import DrivePane from '../drive/DrivePane.vue'
import GitPane from '../git/GitPane.vue'
import { spaceGroup, type DriveScope } from '../../lib/drive'
import type { GitLayout } from '../../lib/gitTree'
import { useRightPaneTab, type RightPaneTab } from '../../stores/useRightPaneTab'
import type { TerminalDriveTarget } from './useTerminalDrive'

const props = defineProps<{
  /** The space the attached session reads; null for a session with none. */
  drive: TerminalDriveTarget | null
  /** The session whose checkout Git reads; null for a pinned chat. */
  gitSession: string | null
  requested: RightPaneTab
  docPath: string | null
  gitPath: string | null
  active: boolean
}>()
const emit = defineEmits<{
  tab: [tab: RightPaneTab]
  close: []
  'pick-doc': [path: string | null]
  'pick-git': [path: string]
  'open-url': [url: string]
  'open-drive-page': [scope: DriveScope]
  setup: []
}>()

// A mismatch resolves to the tab there is, without rewriting the route: the
// query still names what the user last asked for.
const tab = computed<RightPaneTab>(() => {
  if (props.requested === 'git') return props.gitSession !== null ? 'git' : 'drive'
  return props.drive !== null ? 'drive' : 'git'
})
const switchable = computed(() => props.drive !== null && props.gitSession !== null)

const { lastTab } = useRightPaneTab()
// Only a tab the user could have chosen counts, or a session with no drive space
// would turn every later toggle into Git.
watch(
  [tab, switchable],
  () => {
    if (switchable.value) lastTab.value = tab.value
  },
  { immediate: true },
)

const tabOptions: { value: RightPaneTab; label: string }[] = [
  { value: 'drive', label: 'Drive' },
  { value: 'git', label: 'Git' },
]

const layout = useStorage<GitLayout>('hive.git.layout', 'tree')

const {
  size: paneWidth,
  startResize,
  step,
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
    aria-label="Right panel"
    data-testid="session-side-pane"
  >
    <PanelResizeHandle edge="left" name="session-side-pane" :start="startResize" :step="step" />

    <div class="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1">
      <SegmentedControl
        v-if="switchable"
        :model-value="tab"
        :options="tabOptions"
        variant="compact"
        size="sm"
        aria-label="Right panel"
        testid="session-side-pane-tab"
        @update:model-value="emit('tab', $event)"
      />
      <span v-else class="px-1.5 text-small font-semibold text-text">{{ tab === 'git' ? 'Git' : 'Drive' }}</span>
      <div class="min-w-0 flex-1" />
      <IconButton
        v-if="tab === 'git'"
        :label="layout === 'tree' ? 'Show as list' : 'Show as tree'"
        :icon="layout === 'tree' ? IconList : IconListTree"
        data-testid="git-pane-layout"
        @click="layout = layout === 'tree' ? 'flat' : 'tree'"
      />
      <IconButton label="Close panel" :icon="IconX" data-testid="session-side-pane-close" @click="emit('close')" />
    </div>

    <GitPane
      v-if="tab === 'git' && gitSession !== null"
      :session-id="gitSession"
      :path="gitPath"
      :layout="layout"
      :active="active"
      @pick="emit('pick-git', $event)"
    />
    <DrivePane
      v-else-if="drive"
      embedded
      :session="drive.session"
      :space="drive.space"
      :path="docPath"
      :agent-setup="spaceGroup(drive.space) === 'Repositories'"
      @setup="emit('setup')"
      @open-url="emit('open-url', $event)"
      @pick="emit('pick-doc', $event)"
      @open-page="(path) => emit('open-drive-page', { ...drive!, path })"
    />
  </aside>
</template>
