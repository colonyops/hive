<script setup lang="ts">
// Renders an action's declared inputs as a form. Shared by the generic
// invocation dialog and the session-launch dialog, which composes it with its
// own repository/name/agent fields.
import { computed } from 'vue'
import { SelectField, TextareaField, TextField } from '../pipeline/fields'
import type { InputSpec } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/actions/models'
import { type ActionInputValues, inputLabel } from '../lib/actionInputs'

const props = defineProps<{ inputs: InputSpec[]; modelValue: ActionInputValues }>()
const emit = defineEmits<{ 'update:modelValue': [values: ActionInputValues] }>()

const labelled = computed(() =>
  props.inputs.map((spec) => ({
    spec,
    label: inputLabel(spec) + (spec.required ? '' : ' (optional)'),
    options: (spec.options ?? []).map((value) => ({ value, label: value })),
  })),
)

function set(name: string, value: string): void {
  emit('update:modelValue', { ...props.modelValue, [name]: value })
}
</script>

<template>
  <template v-for="{ spec, label, options } in labelled" :key="spec.name">
    <SelectField
      v-if="spec.type === 'select'"
      :label="label"
      :model-value="modelValue[spec.name] ?? ''"
      :options="options"
      :placeholder="spec.placeholder"
      :testid="`action-input-${spec.name}`"
      @update:model-value="set(spec.name, $event)"
    />
    <TextareaField
      v-else-if="spec.type === 'multiline'"
      :label="label"
      :model-value="modelValue[spec.name] ?? ''"
      :placeholder="spec.placeholder"
      :rows="4"
      :testid="`action-input-${spec.name}`"
      @update:model-value="set(spec.name, $event)"
    />
    <TextField
      v-else
      :label="label"
      :model-value="modelValue[spec.name] ?? ''"
      :placeholder="spec.placeholder"
      :testid="`action-input-${spec.name}`"
      @update:model-value="set(spec.name, $event)"
    />
  </template>
</template>
