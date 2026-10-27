<script setup lang="ts">
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import IconLayoutGrid from '~icons/material-symbols/grid-view'
import IconList from '~icons/material-symbols/view-list'
import type { ViewMode } from '../lib/viewMode'

defineProps<{ modelValue: ViewMode }>()
const emit = defineEmits<{ 'update:modelValue': [mode: ViewMode] }>()

const options: { value: ViewMode; label: string; title: string }[] = [
  { value: 'list', label: 'List', title: 'List view' },
  { value: 'grid', label: 'Grid', title: 'Grid view' },
]
</script>

<template>
  <SegmentedControl
    :model-value="modelValue"
    variant="compact"
    size="md"
    aria-label="Layout"
    :options="options"
    testid="view-toggle"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <template #option="{ option }">
      <IconList v-if="option.value === 'list'" class="size-4" aria-hidden="true" />
      <IconLayoutGrid v-else class="size-4" aria-hidden="true" />
    </template>
  </SegmentedControl>
</template>
