<script setup lang="ts">
import { useResizablePanel } from '@hive/ui/composables/useResizablePanel'
import { useStorage } from '@vueuse/core'
import type { ModeSidebar } from '../lib/modeSidebars'

const props = defineProps<{ sidebar: ModeSidebar }>()

// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- a mode's sidebar is a constant
const collapsed = useStorage(props.sidebar.collapsedKey, false)
// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- a mode's sidebar is a constant
const { size } = useResizablePanel(props.sidebar.panel)
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 bg-app" data-testid="mode-skeleton">
    <aside v-if="!collapsed" class="shrink-0 border-r border-border bg-sidebar" :style="{ width: `${size}px` }" />
  </div>
</template>
