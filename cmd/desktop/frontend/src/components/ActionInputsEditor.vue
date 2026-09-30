<script setup lang="ts">
// Declares the inputs an action collects at invocation time. The list is the
// prompt order, so a new input is appended rather than sorted in.
import IconPlus from '~icons/lucide/plus'
import IconTrash from '~icons/lucide/trash-2'
import AppCheckbox from './AppCheckbox.vue'
import { SelectField, TextareaField, TextField } from '../pipeline/fields'
import type { InputSpec } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/actions/models'

const props = defineProps<{ modelValue: InputSpec[] }>()
const emit = defineEmits<{ 'update:modelValue': [inputs: InputSpec[]] }>()

// A literal in the template would be parsed as an interpolation.
const templateExample = '{{ .Inputs.name }}'

const typeOptions = [
  { value: 'text', label: 'Text' },
  { value: 'multiline', label: 'Multiline' },
  { value: 'select', label: 'Select' },
]

function patch(index: number, changes: Partial<InputSpec>): void {
  emit('update:modelValue', props.modelValue.map((spec, i) => (i === index ? { ...spec, ...changes } : spec)))
}

function setType(index: number, type: string): void {
  // Options only mean something on a select, and the backend rejects them
  // anywhere else — so switching away drops them rather than saving a value
  // the catalog will refuse.
  patch(index, { type, options: type === 'select' ? (props.modelValue[index].options ?? []) : null })
}

function optionsText(spec: InputSpec): string { return (spec.options ?? []).join('\n') }
function setOptions(index: number, text: string): void {
  patch(index, { options: text.split('\n').map((option) => option.trim()).filter(Boolean) })
}

function add(): void {
  emit('update:modelValue', [...props.modelValue, { name: '', label: '', type: 'text', required: false, default: '', placeholder: '', options: null }])
}
function remove(index: number): void {
  emit('update:modelValue', props.modelValue.filter((_, i) => i !== index))
}
</script>

<template>
  <div class="grid gap-2">
    <div class="flex items-center justify-between">
      <div class="text-[12.5px] text-text-2">Inputs</div>
      <button class="flex items-center gap-1 text-[12px] text-text-3 hover:text-text" data-testid="action-input-add" @click="add"><IconPlus class="size-3.5" />Add input</button>
    </div>
    <p v-if="!modelValue.length" class="text-xs leading-relaxed text-text-4">Values collected when the action runs, available to its templates as <code class="font-mono">{{ templateExample }}</code>.</p>
    <div v-for="(spec, index) in modelValue" :key="index" class="grid gap-2.5 rounded-lg border border-row p-3">
      <div class="flex items-start gap-2">
        <div class="flex-1"><TextField :model-value="spec.name" label="Name" monospace :testid="`action-input-name-${index}`" @update:model-value="patch(index, { name: $event })" /></div>
        <button class="mt-7 text-text-3 hover:text-severity-error" :aria-label="`Remove input ${index + 1}`" :data-testid="`action-input-remove-${index}`" @click="remove(index)"><IconTrash class="size-4" /></button>
      </div>
      <TextField :model-value="spec.label" label="Label" :testid="`action-input-label-${index}`" @update:model-value="patch(index, { label: $event })" />
      <SelectField :model-value="spec.type" label="Type" :options="typeOptions" :testid="`action-input-type-${index}`" @update:model-value="setType(index, $event)" />
      <TextareaField
        v-if="spec.type === 'select'"
        :model-value="optionsText(spec)"
        label="Options (one per line)"
        monospace
        :testid="`action-input-options-${index}`"
        @update:model-value="setOptions(index, $event)"
      />
      <TextField :model-value="spec.default" label="Default" :testid="`action-input-default-${index}`" @update:model-value="patch(index, { default: $event })" />
      <TextField v-if="spec.type !== 'select'" :model-value="spec.placeholder" label="Placeholder" :testid="`action-input-placeholder-${index}`" @update:model-value="patch(index, { placeholder: $event })" />
      <AppCheckbox :model-value="spec.required" label="Required" :testid="`action-input-required-${index}`" @update:model-value="patch(index, { required: $event })" />
    </div>
  </div>
</template>
