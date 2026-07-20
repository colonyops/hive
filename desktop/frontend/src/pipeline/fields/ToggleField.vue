<script setup lang="ts">
import AppCheckbox from '../../components/AppCheckbox.vue'

defineProps<{ label?: string; modelValue: boolean; hint?: string; testid?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <AppCheckbox :model-value="modelValue" :label="label" :hint="hint" :testid="testid" @update:model-value="(value) => emit('update:modelValue', value)" />
</template>
