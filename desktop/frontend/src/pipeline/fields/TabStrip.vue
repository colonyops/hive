<script setup lang="ts">
// A small segmented tab switcher — used by the function node's editor to
// flip between on_start/on_message/on_stop without three always-visible
// code fields.
export interface TabOption {
  value: string
  label: string
}

const props = defineProps<{
  modelValue: string
  tabs: TabOption[]
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <div class="flex gap-1 rounded-lg border border-row bg-app p-1" role="tablist">
    <button
      v-for="tab in tabs"
      :key="tab.value"
      type="button"
      role="tab"
      class="flex-1 cursor-pointer rounded-md px-2.5 py-1.5 text-[12.5px] transition-colors"
      :class="modelValue === tab.value ? 'bg-raised text-text' : 'text-text-3 hover:text-text-2'"
      :aria-selected="modelValue === tab.value"
      :data-testid="testid ? `${testid}-${tab.value}` : undefined"
      @click="emit('update:modelValue', tab.value)"
    >{{ tab.label }}</button>
  </div>
</template>
