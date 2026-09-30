<script setup lang="ts">
// A large panel over whatever view is active. It is deliberately the same size
// as TasksOverlay — the two are siblings in the chrome, opened from adjacent
// title-bar icons, so a different footprint made switching between them look
// like a layout jump. Change one and change the other.
// Reuses BaseModal's mechanics (teleport, backdrop, focus trap, return focus)
// rather than BaseModal itself: its fixed pixel width and title-only header
// don't fit a toolbar'd view. ActivityView already calls useEscapeToClose, so
// Escape is handled there and not duplicated here.
//
// This overlay must never register with useOpenModalCount (so also never become
// BaseModal-backed, which registers on mount): App.vue reads a non-zero count
// as "a dialog is stacked above the overlay", and the tasks overlay beside it
// would then permanently refuse to close.
import { ref } from 'vue'
import ActivityView from './ActivityView.vue'
import { useAutofocus } from '../composables/useAutofocus'
import { useFocusTrap } from '../composables/useFocusTrap'
import { useReturnFocus } from '../composables/useReturnFocus'
import type { ActivityItemLink } from '../lib/activityPresentation'

const emit = defineEmits<{
  close: []
  'open-url': [url: string]
  'open-item': [item: ActivityItemLink]
}>()

const panel = ref<HTMLElement | null>(null)

function close(): void {
  emit('close')
}

function openItem(item: ActivityItemLink): void {
  emit('open-item', item)
  close()
}

const { onKeydown: trapFocus } = useFocusTrap(panel)
useReturnFocus()
// Opened over a terminal, focus is still on the pane's textarea. The panel
// takes focus itself rather than the search box, which is editable and would
// swallow keys meant for the surface underneath.
useAutofocus(panel)
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-40 flex items-center justify-center bg-backdrop py-[6vh]"
      data-testid="activity-overlay-backdrop"
      @click.self="close"
    >
      <div
        ref="panel"
        class="flex h-[88vh] w-[min(1600px,96vw)] flex-col overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl"
        role="dialog"
        aria-label="Activity"
        aria-modal="true"
        data-testid="activity-overlay"
        tabindex="-1"
        @keydown="trapFocus"
      >
        <ActivityView @close="close" @open-url="emit('open-url', $event)" @open-item="openItem" />
      </div>
    </div>
  </Teleport>
</template>
