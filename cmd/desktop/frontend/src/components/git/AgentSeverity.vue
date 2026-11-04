<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ severity: string; count?: number }>()

const STYLES: Record<string, string> = {
  issue: 'bg-severity-error-tint text-severity-error',
  suggestion: 'bg-severity-info-tint text-severity-info',
  note: 'bg-chip text-text-2',
}
const PLURALS: Record<string, string> = { issue: 'issues', suggestion: 'suggestions', note: 'notes' }
const text = computed(() =>
  props.count === undefined
    ? props.severity
    : `${props.count} ${props.count === 1 ? props.severity : (PLURALS[props.severity] ?? props.severity)}`,
)
</script>

<template>
  <span
    class="shrink-0 rounded-sm px-1 py-px font-mono text-micro font-semibold tracking-wide uppercase"
    :class="STYLES[severity] ?? STYLES.note"
    >{{ text }}</span
  >
</template>
