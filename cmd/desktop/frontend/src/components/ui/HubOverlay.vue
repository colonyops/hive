<script setup lang="ts">
// The large panel the Tasks and Activity views open in. It is not a BaseModal:
// a hub view brings its own toolbar and Escape handling, and it must never
// register with useOpenModalCount, because App.vue and TasksView read a
// non-zero count as "a dialog is stacked above the overlay" and would never
// let it close.
import { ref } from 'vue'
import { useAutofocus } from '../../composables/useAutofocus'
import { useFocusTrap } from '../../composables/useFocusTrap'
import { useReturnFocus } from '../../composables/useReturnFocus'

defineProps<{ label: string; testid: string }>()
const emit = defineEmits<{ close: [] }>()

const panel = ref<HTMLElement | null>(null)
const { onKeydown: trapFocus } = useFocusTrap(panel)
useReturnFocus()
// Opened over a terminal, focus is still in the pane's textarea and keys
// would keep typing into the shell. The panel takes focus rather than a
// search box, which would swallow the view's keys the same way.
useAutofocus(panel)
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-40 flex items-center justify-center bg-backdrop py-[6vh]"
      :data-testid="`${testid}-backdrop`"
      @click.self="emit('close')"
    >
      <div
        ref="panel"
        class="flex h-[88vh] w-[min(1600px,96vw)] flex-col overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl"
        role="dialog"
        :aria-label="label"
        aria-modal="true"
        :data-testid="testid"
        tabindex="-1"
        @keydown="trapFocus"
      >
        <slot />
      </div>
    </div>
  </Teleport>
</template>
