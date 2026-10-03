<script setup lang="ts">
// FormField chrome around the app-wide AppSelect listbox. Everything about the
// control itself — keyboard handling, popover placement, search, icons — lives
// in AppSelect; this only adds the label/hint/error layout the field kit shares.
import AppSelect, { type AppSelectOption } from '../../components/ui/AppSelect.vue'
import FormField from '../../components/ui/FormField.vue'

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
  <FormField v-slot="{ id }" :label="label" :hint="hint" :error="error" :testid="testid">
    <AppSelect
      :id="id"
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
  </FormField>
</template>
