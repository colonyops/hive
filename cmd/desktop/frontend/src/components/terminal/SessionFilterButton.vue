<script setup lang="ts">
import { ref, shallowRef } from 'vue'
import IconListFilter from '~icons/lucide/list-filter'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useTerminalModeContext } from './terminalModeContext'

const { viewMenus } = useTerminalModeContext()

const open = ref(false)
const wrap = shallowRef<HTMLElement | null>(null)

function onSelect(id: string): void {
  open.value = false
  viewMenus.onFilterSelect(id)
}
</script>

<template>
  <div ref="wrap" class="relative flex">
    <IconButton
      label="Filter sessions by…"
      :icon="IconListFilter"
      :active="open"
      data-testid="code-view-add-filter"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click="open = !open"
    />
    <!-- Start-aligned: the sidebar sits at the window's left edge, so an end-aligned panel would leave the screen. -->
    <AppMenu
      v-if="open"
      align="start"
      :entries="viewMenus.filterEntries"
      :ignore="[wrap]"
      width="12rem"
      testid="code-view-add-menu"
      @close="open = false"
      @select="onSelect"
    />
  </div>
</template>
