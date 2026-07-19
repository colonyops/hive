<script setup lang="ts">
import SettingsField from './SettingsField.vue'

export interface SettingsSelectOption {
  value: string
  label: string
}

const props = defineProps<{
  label?: string
  modelValue: string
  options: SettingsSelectOption[]
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onChange(e: Event) {
  emit('update:modelValue', (e.target as HTMLSelectElement).value)
}
</script>

<template>
  <SettingsField :label="label" :hint="hint" :testid="testid">
    <select
      :id="testid"
      :value="modelValue"
      class="w-full cursor-pointer rounded-lg border border-strong bg-app px-3.5 py-2.5 text-[13.5px] text-text outline-none focus:border-accent"
      :data-testid="testid"
      @change="onChange"
    >
      <option v-for="opt in options" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
    </select>
  </SettingsField>
</template>
