<script setup lang="ts">
import { useRemoteCatalog } from '../stores/useRemoteCatalog'
import AgentsMode from './AgentsMode.vue'
import RemoteChats from './RemoteChats.vue'
import type { DriveScope } from '../lib/drive'
defineProps<{ active: boolean; sidebarCollapsed?: boolean }>()
const emit = defineEmits<{ 'open-drive-page': [scope: DriveScope] }>()
const { workspace } = useRemoteCatalog()
</script>
<template>
  <div class="flex min-h-0 min-w-0 flex-1">
    <AgentsMode
      v-show="!workspace"
      :active="active && !workspace"
      :sidebar-collapsed="sidebarCollapsed"
      @open-drive-page="emit('open-drive-page', $event)"
    />
    <RemoteChats v-if="workspace" v-show="active" :active="active" />
  </div>
</template>
