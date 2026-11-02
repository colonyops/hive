<script setup lang="ts">
import { ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import SessionAddMenu from './SessionAddMenu.vue'

defineProps<{
  /** The session a "+" adds to, once its row has landed. */
  row: TerminalSessionRow | null
  panel: string
}>()

const menuOpen = ref(false)
const addButton = ref<HTMLElement | null>(null)
</script>

<template>
  <div ref="addButton" class="relative flex items-center">
    <IconButton
      label="Open a tab"
      :icon="IconPlus"
      size="sm"
      :active="menuOpen"
      :data-testid="`panel-tab-add-${panel}`"
      @click="menuOpen = !menuOpen"
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
