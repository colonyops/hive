<script setup lang="ts">
// FieldRow chrome around the app-wide AppSelect listbox. Everything about the
// control itself — keyboard handling, popover placement, search, icons — lives
// in AppSelect; this only adds the label/hint/error layout the field kit shares.
import AppSelect, { type AppSelectOption } from '../../components/AppSelect.vue'
import FieldRow from './FieldRow.vue'

export type SelectOption = AppSelectOption

defineProps<{
  label?: string
  modelValue: string
  options: SelectOption[]
  placeholder?: string
  searchable?: boolean
  searchPlaceholder?: string
  disabled?: boolean
  hint?: string
  error?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <AppSelect
      :model-value="modelValue"
      :options="options"
      :placeholder="placeholder"
      :searchable="searchable"
      :search-placeholder="searchPlaceholder"
      :disabled="disabled"
      :aria-label="label"
      :testid="testid"
      @update:model-value="emit('update:modelValue', $event)"
    />
  </FieldRow>
</template>
