<script setup lang="ts">
import { useCodeInstallation } from '../stores/useCodeInstallation'
import TerminalMode from './TerminalMode.vue'
import RemoteCode from './RemoteCode.vue'
import type { TerminalDriveTarget } from './terminal/useTerminalDrive'
import type { DriveScope } from '../lib/drive'

withDefaults(defineProps<{ active?: boolean; sidebarCollapsed?: boolean; drive?: TerminalDriveTarget | null }>(), {
  active: true,
  drive: null,
})
const emit = defineEmits<{
  'open-tasks': []
  'session-repo-key': [repoKey: string]
  'open-drive-page': [scope: DriveScope]
}>()
const { installation } = useCodeInstallation()
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <TerminalMode
      :active="active && installation === 'local'"
      :sidebar-collapsed="sidebarCollapsed"
      :drive="drive"
      :remote="installation === 'remote'"
      @open-tasks="emit('open-tasks')"
      @session-repo-key="emit('session-repo-key', $event)"
      @open-drive-page="emit('open-drive-page', $event)"
    >
      <RemoteCode :active="active && installation === 'remote'" />
    </TerminalMode>
  </div>
</template>
