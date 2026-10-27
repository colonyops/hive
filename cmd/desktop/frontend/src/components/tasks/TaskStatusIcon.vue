<script setup lang="ts">
import { computed } from 'vue'
import { statusMeta } from '../../lib/tasksPresentation'

const props = defineProps<{ status: string }>()

const meta = computed(() => statusMeta(props.status))
const filled = computed(() => props.status === 'done' || props.status === 'cancelled')
</script>

<template>
  <svg
    viewBox="0 0 14 14"
    class="size-3.5 shrink-0"
    :class="meta.iconClass"
    role="img"
    :aria-label="meta.label"
    :data-status="status"
  >
    <circle v-if="filled" cx="7" cy="7" r="6.5" fill="currentColor" />
    <circle v-else cx="7" cy="7" r="5.75" fill="none" stroke="currentColor" stroke-width="1.5" />
    <path v-if="status === 'in_progress'" d="M7 3.5 A3.5 3.5 0 0 1 7 10.5 Z" fill="currentColor" />
    <path
      v-else-if="status === 'done'"
      d="M4.25 7.25 6.1 9 9.75 5.25"
      fill="none"
      class="stroke-pane"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <path
      v-else-if="status === 'cancelled'"
      d="M5 5 9 9 M9 5 5 9"
      fill="none"
      class="stroke-pane"
      stroke-width="1.5"
      stroke-linecap="round"
    />
  </svg>
</template>
