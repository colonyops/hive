<script setup lang="ts">
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useCodeInstallation } from '../stores/useCodeInstallation'
import TerminalMode from './TerminalMode.vue'
import RemoteCode from './RemoteCode.vue'
import SegmentedControl from './ui/SegmentedControl.vue'
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
const { installation, select } = useCodeInstallation()
const { activeProfile, color } = useRemoteConnection()
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
      >
        <template #option="{ option }">
          <span class="flex items-center gap-2">
            <span
              v-if="option.value === 'remote' && activeProfile"
              class="size-2.5 rounded-full border border-strong"
              :style="{ backgroundColor: color }"
              aria-hidden="true"
            />
            {{ option.value === 'remote' && activeProfile ? activeProfile.name : option.label }}
          </span>
        </template>
      </SegmentedControl>
    </div>
    <TerminalMode
      v-show="installation === 'local'"
      :active="active && installation === 'local'"
      :sidebar-collapsed="sidebarCollapsed"
      :drive="drive"
      @open-tasks="emit('open-tasks')"
      @session-repo-key="emit('session-repo-key', $event)"
      @open-drive-page="emit('open-drive-page', $event)"
    />
    <RemoteCode v-show="installation === 'remote'" :active="active && installation === 'remote'" />
  </div>
</template>
