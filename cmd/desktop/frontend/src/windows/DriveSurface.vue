<script setup lang="ts">
import { useStorage } from '@vueuse/core'
import { computed, ref } from 'vue'
import IconPanelLeft from '~icons/lucide/panel-left'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import DrivePage from '../components/drive/DrivePage.vue'
import { useOpenUrl } from '../composables/useOpenUrl'
import { driveScopeQuery, type DriveScope } from '../lib/drive'
import PopOutTitleBar from './PopOutTitleBar.vue'

const props = defineProps<{ params: Record<string, string> }>()
const emit = defineEmits<{ 'update:params': [params: Record<string, string>]; title: [title: string] }>()

const openUrl = useOpenUrl()

const scope = computed<DriveScope>(() => ({
  space: props.params.space ?? '',
  path: props.params.doc || null,
  session: props.params.session || null,
}))

// Starts on the document alone: a pop-out is for reading beside something else.
const sidebarOpen = useStorage('hive.popout.drive.sidebar', false)

const title = ref('Drive')
function updateTitle(next: string): void {
  title.value = next
  emit('title', next)
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="popout-drive">
    <PopOutTitleBar :title="title">
      <IconButton
        :label="sidebarOpen ? 'Hide files' : 'Show files'"
        :icon="IconPanelLeft"
        :active="sidebarOpen"
        :aria-pressed="sidebarOpen"
        data-testid="popout-drive-sidebar"
        @click="sidebarOpen = !sidebarOpen"
      />
    </PopOutTitleBar>
    <div class="flex min-h-0 flex-1">
      <DrivePage
        windowed
        :scope="scope"
        :sidebar-collapsed="!sidebarOpen"
        @update:scope="emit('update:params', driveScopeQuery($event))"
        @open-url="openUrl"
        @title="updateTitle"
      />
    </div>
  </div>
</template>
