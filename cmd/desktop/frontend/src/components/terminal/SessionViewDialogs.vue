<script setup lang="ts">
import IconLayers from '~icons/lucide/layers'
import RenameDialog from '@hive/ui/components/ui/RenameDialog.vue'
import SessionViewsDrawer from './SessionViewsDrawer.vue'
import { useTerminalModeContext } from './terminalModeContext'

const { viewMenus } = useTerminalModeContext()
</script>

<template>
  <RenameDialog
    v-if="viewMenus.saving"
    title="Save view"
    label="Name"
    confirm-label="Save"
    :icon="IconLayers"
    testid="code-view-save-dialog"
    @close="viewMenus.saving = false"
    @save="viewMenus.onSaveAs"
  />
  <SessionViewsDrawer v-if="viewMenus.managing" @close="viewMenus.managing = false" />
</template>
