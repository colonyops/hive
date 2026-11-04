<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ severity: string; count?: number }>()

const PLURALS: Record<string, string> = { issue: 'issues', suggestion: 'suggestions', note: 'notes' }
const text = computed(() =>
  props.count === undefined
    ? props.severity
    : `${props.count} ${props.count === 1 ? props.severity : (PLURALS[props.severity] ?? props.severity)}`,
)
</script>

<template>
  <span class="shrink-0 text-caption text-text-3">{{ text }}</span>
</template>
