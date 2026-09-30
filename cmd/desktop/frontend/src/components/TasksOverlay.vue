<script setup lang="ts">
// A large panel over whatever view is active, sized for TasksView's
// tree+detail split. ActivityOverlay matches it deliberately — change one and
// change the other. Reuses BaseModal's mechanics (teleport, backdrop, focus
// trap, return focus) rather than BaseModal itself: its fixed pixel width and
// title-only header don't fit a toolbar'd hub view. TasksView already calls
// useEscapeToClose, so Escape is handled there and not duplicated here.
//
// This overlay must never register with useOpenModalCount (so also never
// become BaseModal-backed, which registers on mount): TasksView's Escape
// handler and App.vue's tasks.toggle exception both read a non-zero count as
// "a dialog is stacked above the overlay" and would permanently refuse to
// close it.
import { ref } from 'vue'
import TasksView from './TasksView.vue'
import { useAutofocus } from '../composables/useAutofocus'
import { useFocusTrap } from '../composables/useFocusTrap'
import { useReturnFocus } from '../composables/useReturnFocus'

const emit = defineEmits<{ close: [] }>()

const panel = ref<HTMLElement | null>(null)

function close(): void {
  emit('close')
}

const { onKeydown: trapFocus } = useFocusTrap(panel)
useReturnFocus()
// Opened over a terminal, focus is still on the pane's textarea, and
// TasksView's j/k walk defers to any editable target, so the keys would keep
// typing into the shell. The panel takes focus itself rather than the search
// box, which is editable too and would swallow the walk the same way.
useAutofocus(panel)
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-40 flex items-center justify-center bg-backdrop py-[6vh]"
      data-testid="tasks-overlay-backdrop"
      @click.self="close"
    >
      <div
        ref="panel"
        class="flex h-[88vh] w-[min(1600px,96vw)] flex-col overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl"
        role="dialog"
        aria-label="Tasks"
        aria-modal="true"
        data-testid="tasks-overlay"
        tabindex="-1"
        @keydown="trapFocus"
      >
        <TasksView @close="close" />
      </div>
    </div>
  </Teleport>
</template>
