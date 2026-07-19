<script setup lang="ts">
import FieldRow from './FieldRow.vue'

export interface SelectOption {
  value: string
  label: string
}

const props = defineProps<{
  label?: string
  modelValue: string
  options: SelectOption[]
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onChange(e: Event) {
  emit('update:modelValue', (e.target as HTMLSelectElement).value)
}
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <select
      :value="modelValue"
      class="w-full cursor-pointer rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none focus:border-accent"
      :data-testid="testid"
      @change="onChange"
    >
      <option v-if="placeholder" value="" disabled>{{ placeholder }}</option>
      <option v-for="opt in options" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
    </select>
  </FieldRow>
</template>
