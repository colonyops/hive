<script setup lang="ts">
import FieldRow from './FieldRow.vue'

const props = defineProps<{
  label?: string
  modelValue: number
  min?: number
  max?: number
  step?: number
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: number] }>()

function onInput(e: Event) {
  const n = Number((e.target as HTMLInputElement).value)
  emit('update:modelValue', Number.isFinite(n) ? n : 0)
}
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <input
      type="number"
      :value="modelValue"
      :min="min"
      :max="max"
      :step="step"
      :placeholder="placeholder"
      class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
      :data-testid="testid"
      @input="onInput"
    >
  </FieldRow>
</template>
