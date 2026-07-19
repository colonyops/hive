<script setup lang="ts">
import SettingsField from './SettingsField.vue'

const props = defineProps<{
  label?: string
  modelValue: string
  placeholder?: string
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).value)
}
</script>

<template>
  <SettingsField :label="label" :hint="hint" :testid="testid">
    <input
      :id="testid"
      type="text"
      :value="modelValue"
      :placeholder="placeholder"
      class="w-full rounded-lg border border-strong bg-app px-3.5 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
      :data-testid="testid"
      @input="onInput"
    >
  </SettingsField>
</template>
