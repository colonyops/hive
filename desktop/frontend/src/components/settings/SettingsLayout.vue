<script setup lang="ts">
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import ViewHeader from './ViewHeader.vue'

const props = defineProps<{
  closeTestid?: string
}>()
const emit = defineEmits<{ close: [] }>()

function close(): void {
  emit('close')
}

useEscapeToClose(close)
</script>

<template>
  <div class="flex h-full min-h-0 flex-1">
    <aside class="hive-scroll w-[200px] shrink-0 overflow-y-auto border-r border-row bg-sidebar">
      <div class="border-b border-border px-4 pb-3 pt-4">
        <slot name="sidebar-title" />
      </div>
      <nav class="flex flex-col gap-0.5 px-2.5 py-3">
        <slot name="nav" />
      </nav>
    </aside>

    <section class="flex min-w-0 flex-1 flex-col">
      <ViewHeader :close-testid="props.closeTestid" @close="close">
        <template #title><slot name="header-title" /></template>
      </ViewHeader>
      <slot />
    </section>
  </div>
</template>
