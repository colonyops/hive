<script setup lang="ts">
// A segmented control — same visual as pipeline/fields/TabStrip.vue (a
// bordered strip of equal-width buttons, the active one raised), kept as a
// local copy because settings pages are independent of the pipeline module.
import SettingsField from './SettingsField.vue'

export interface SettingsSegmentedOption {
  value: string
  label: string
  /** Spelled-out name when the label is abbreviated to keep the strip narrow. */
  title?: string
}

const props = defineProps<{
  label?: string
  modelValue: string
  options: SettingsSegmentedOption[]
  hint?: string
  testid?: string
  /** Names the strip when a SettingsRow supplies the visible label instead. */
  ariaLabel?: string
  /** Lay the options out as a grid with this many columns instead of a single strip. */
  columns?: number
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <SettingsField :label="label" :hint="hint" :testid="testid">
    <div
      class="gap-1 rounded-lg border border-row bg-app p-1"
      :class="props.columns ? 'grid' : 'flex'"
      :style="props.columns ? { gridTemplateColumns: `repeat(${props.columns}, minmax(0, 1fr))` } : undefined"
      role="tablist"
      :aria-label="props.ariaLabel ?? props.label"
    >
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        role="tab"
        class="flex-1 cursor-pointer whitespace-nowrap rounded-md px-2.5 py-1.5 text-[12.5px] transition-colors"
        :class="modelValue === opt.value ? 'bg-raised text-text' : 'text-text-3 hover:text-text-2'"
        :aria-selected="modelValue === opt.value"
        :title="opt.title"
        :data-testid="testid ? `${testid}-${opt.value}` : undefined"
        @click="emit('update:modelValue', opt.value)"
      >{{ opt.label }}</button>
    </div>
  </SettingsField>
</template>
