<script setup lang="ts">
import { computed } from 'vue'
import ToastCard from './ToastCard.vue'
import type { ToastInstance } from '../types/toast'

const props = defineProps<{ toasts: ToastInstance[] }>()
const emit = defineEmits<{ dismiss: [id: number]; 'clear-all': [] }>()

// design spec "6a Toasts": overflow beyond ~4 visible toasts collapses into
// an "N more · Clear all" footer instead of growing the stack unbounded.
// Newest toasts (end of the array — showToast appends) stay visible; older
// ones roll into the overflow count.
const MAX_VISIBLE = 4

const visible = computed(() => props.toasts.slice(-MAX_VISIBLE))
const overflowCount = computed(() => Math.max(0, props.toasts.length - MAX_VISIBLE))
</script>

<template>
  <div v-if="toasts.length" class="fixed bottom-[22px] right-[22px] z-40 flex w-[376px] flex-col gap-3" data-testid="toast-stack">
    <TransitionGroup name="toast">
      <ToastCard
        v-for="toast in visible"
        :key="toast.id"
        :toast="toast"
        @dismiss="emit('dismiss', toast.id)"
      />
    </TransitionGroup>
    <div v-if="overflowCount > 0" class="text-center font-mono text-[11.5px] text-text-3" data-testid="toast-overflow">
      {{ overflowCount }} more · <button class="cursor-pointer hover:text-text" data-testid="toast-clear-all" @click="emit('clear-all')">Clear all</button>
    </div>
  </div>
</template>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: opacity .16s ease, transform .16s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(5px); }
.toast-leave-active { position: absolute; }
</style>
