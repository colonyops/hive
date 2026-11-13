<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { TerminalPane } from '../composables/useTerminalWindows'
import { useNativeRenderer } from '../stores/useNativeRenderer'
import NativeTerminalSurface from './terminal/NativeTerminalSurface.vue'

const props = defineProps<{
  pane: TerminalPane
  active: boolean
  scrollbarWidth: number
  /** The window's active pane, which takes the keyboard. */
  current: boolean
  /** The session's terminal target, which a native surface reads the pane from. */
  slug: string
  /** The pane's window is in front, so a native surface may show. */
  onScreen: boolean
}>()
const emit = defineEmits<{ mount: [paneId: string, host: HTMLElement]; select: [paneId: string] }>()

const host = ref<HTMLElement | null>(null)
const native = useNativeRenderer()

// The bridge ends when its stream does (a dropped socket, an app reload) while
// tmux still has the pane; a fresh surface reattaches it.
const surfaceGeneration = ref(0)
function reopen(): void {
  setTimeout(() => surfaceGeneration.value++, 500)
}

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
    <div v-if="native.enabled.value" class="absolute inset-0 flex">
      <NativeTerminalSurface
        :id="`pane:${slug}:${pane.paneId}:${surfaceGeneration}`"
        :key="surfaceGeneration"
        :target="{ slug, paneId: pane.paneId }"
        :on-screen="onScreen"
        :wants-focus="onScreen && current"
        :overlay-open="false"
        @focused="emit('select', pane.paneId)"
        @exited="reopen"
      />
    </div>
  </div>
</template>

<style scoped>
.terminal-pane-scrollbar :deep(.xterm-scrollable-element > .scrollbar.vertical) {
  display: block;
}
</style>
