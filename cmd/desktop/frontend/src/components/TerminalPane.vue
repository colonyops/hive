<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { TerminalPane } from '../composables/useTerminalWindows'

const props = defineProps<{ pane: TerminalPane; active: boolean; scrollbarWidth: number }>()
const emit = defineEmits<{ mount: [paneId: string, host: HTMLElement]; select: [paneId: string] }>()

const host = ref<HTMLElement | null>(null)

onMounted(() => {
  if (host.value) emit('mount', props.pane.paneId, host.value)
})
</script>

<template>
  <!-- Select on the pane box, not xterm's whole-cell screen, so the fractional
       remainder also accepts focus. -->
  <div
    class="terminal-pane absolute overflow-hidden"
    :class="{ 'terminal-pane-scrollbar': scrollbarWidth > 0 }"
    data-testid="terminal-pane-host"
    :data-pane-id="pane.paneId"
    :data-active-pane="active ? 'true' : undefined"
    @mousedown="emit('select', pane.paneId)"
  >
    <div ref="host" class="size-full" />
  </div>
</template>

<style scoped>
.terminal-pane-scrollbar :deep(.xterm-scrollable-element > .scrollbar.vertical) {
  display: block;
}
</style>
