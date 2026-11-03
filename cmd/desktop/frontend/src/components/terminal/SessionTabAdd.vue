<script setup lang="ts">
import { ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import SessionAddMenu from './SessionAddMenu.vue'
import { useTerminalModeContext } from './terminalModeContext'

const props = defineProps<{
  /** The session a "+" adds to, once its row has landed. */
  row: TerminalSessionRow | null
  panel: string
}>()

const { tree, panels } = useTerminalModeContext()
const menuOpen = ref(false)
const addButton = ref<HTMLElement | null>(null)

// The scratch terminal holds shells and nothing else, so its "+" opens one
// straight away, as its row in the tree does.
function add(): void {
  if (props.row && tree.isScratch(props.row)) void panels.newWindow(props.row, props.panel)
  else menuOpen.value = !menuOpen.value
}
</script>

<template>
  <div ref="addButton" class="relative flex items-center">
    <IconButton
      label="Open a tab"
      :icon="IconPlus"
      size="sm"
      :active="menuOpen"
      :data-testid="`panel-tab-add-${panel}`"
      @click="add"
    />
    <SessionAddMenu
      v-if="menuOpen && row"
      :row="row"
      :panel="panel"
      :anchor="addButton"
      :ignore="[addButton]"
      @close="menuOpen = false"
    />
  </div>
</template>
