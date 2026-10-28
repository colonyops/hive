<script setup lang="ts">
import IconLayers from '~icons/lucide/layers'
import IconListFilter from '~icons/lucide/list-filter'
import RenameDialog from '@hive/ui/components/ui/RenameDialog.vue'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const { viewMenus } = useTerminalModeContext()
const { activeView } = useCodeViews()
</script>

<template>
  <RenameDialog
    v-if="viewMenus.dialog?.kind === 'save-as'"
    title="Save view"
    label="Name"
    confirm-label="Save"
    :icon="IconLayers"
    testid="code-view-save-dialog"
    @close="viewMenus.dialog = null"
    @save="viewMenus.onDialogSave"
  />
  <RenameDialog
    v-else-if="viewMenus.dialog?.kind === 'rename'"
    title="Rename view"
    label="Name"
    :name="activeView?.name ?? ''"
    testid="code-view-rename-dialog"
    @close="viewMenus.dialog = null"
    @save="viewMenus.onDialogSave"
  />
  <RenameDialog
    v-else-if="viewMenus.dialog?.kind === 'text'"
    title="Filter by text"
    label="Matches name, slug, or tag"
    :name="viewMenus.textDialogSeed"
    :confirm-label="viewMenus.dialog.index < 0 ? 'Add' : 'Update'"
    :icon="IconListFilter"
    testid="code-view-text-dialog"
    @close="viewMenus.dialog = null"
    @save="viewMenus.onDialogSave"
  />
</template>
