<script setup lang="ts">
import { useCodeInstallation } from '../stores/useCodeInstallation'
import TerminalMode from './TerminalMode.vue'
import RemoteCode from './RemoteCode.vue'
import SegmentedControl from './ui/SegmentedControl.vue'
import type { TerminalCanvasTarget } from './terminal/useTerminalCanvas'
import type { CanvasScope } from '../lib/agentCanvas'

withDefaults(defineProps<{ active?: boolean; sidebarCollapsed?: boolean; canvas?: TerminalCanvasTarget | null }>(), {
  active: true,
  canvas: null,
})
const emit = defineEmits<{
  'open-tasks': []
  'session-repo-key': [repoKey: string]
  'open-canvas-page': [scope: CanvasScope]
}>()
const { installation, select } = useCodeInstallation()
const options = [
  { value: 'local' as const, label: 'Local' },
  { value: 'remote' as const, label: 'Remote (preview)' },
]
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <div class="flex shrink-0 items-center border-b border-border px-3 py-2">
      <SegmentedControl
        :model-value="installation"
        @update:model-value="select"
        :options="options"
        variant="compact"
        aria-label="Code installation"
        testid="code-installation"
      />
    </div>
    <TerminalMode
      v-show="installation === 'local'"
      :active="active && installation === 'local'"
      :sidebar-collapsed="sidebarCollapsed"
      :canvas="canvas"
      @open-tasks="emit('open-tasks')"
      @session-repo-key="emit('session-repo-key', $event)"
      @open-canvas-page="emit('open-canvas-page', $event)"
    />
    <RemoteCode v-show="installation === 'remote'" :active="active && installation === 'remote'" />
  </div>
</template>
