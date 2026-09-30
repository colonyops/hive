<script setup lang="ts">
// The cadence floor every pull source node offers. It is its own field rather
// than a TextField per editor so eight source editors carry one wording, and
// so the copy moves in one place when the poll contract does.
import TextField from './TextField.vue'
import { INTERVAL_HINT, INTERVAL_LABEL, INTERVAL_PLACEHOLDER } from '../lib/sourceInterval'

defineProps<{ modelValue?: string; testid?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string | undefined] }>()

// Empty is "every tick", which the config expresses by omitting the key —
// an empty string would write `interval: ""` into the flow's YAML.
function onUpdate(value: string): void {
  emit('update:modelValue', value.trim() || undefined)
}
</script>

<template>
  <TextField
    :label="INTERVAL_LABEL"
    :model-value="modelValue ?? ''"
    :placeholder="INTERVAL_PLACEHOLDER"
    :hint="INTERVAL_HINT"
    monospace
    :testid="testid"
    @update:model-value="onUpdate"
  />
</template>
