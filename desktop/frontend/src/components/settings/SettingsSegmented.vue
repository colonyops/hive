<script setup lang="ts">
// A segmented control — same visual as pipeline/fields/TabStrip.vue (a
// bordered strip of equal-width buttons, the active one raised), kept as a
// local copy because settings pages are independent of the pipeline module.
import SettingsField from './SettingsField.vue'

export interface SettingsSegmentedOption {
  value: string
  label: string
}

const props = defineProps<{
  label?: string
  modelValue: string
  options: SettingsSegmentedOption[]
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <SettingsField :label="label" :hint="hint" :testid="testid">
    <div class="flex gap-1 rounded-lg border border-row bg-app p-1" role="tablist" :aria-label="label">
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        role="tab"
        class="flex-1 cursor-pointer whitespace-nowrap rounded-md px-2.5 py-1.5 text-[12.5px] transition-colors"
        :class="modelValue === opt.value ? 'bg-raised text-text' : 'text-text-3 hover:text-text-2'"
        :aria-selected="modelValue === opt.value"
        :data-testid="testid ? `${testid}-${opt.value}` : undefined"
        @click="emit('update:modelValue', opt.value)"
      >{{ opt.label }}</button>
    </div>
  </SettingsField>
</template>
