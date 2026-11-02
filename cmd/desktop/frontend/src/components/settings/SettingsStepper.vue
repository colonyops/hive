<script setup lang="ts">
// SegmentedControl's strip for a setting whose range is open rather than a
// handful of named options, where it would need a button per rung.
import FormField from '@hive/ui/components/ui/FormField.vue'

const props = defineProps<{
  label?: string
  modelValue: number
  /** The value as the row shows it — `14px`, not `14`. */
  display: string
  min: number
  max: number
  /** The next value in a direction; replaces `step` for a setting with uneven rungs. */
  next?: (value: number, direction: 1 | -1) => number
  step?: number
  hint?: string
  testid?: string
  /** Names the strip when a SettingsRow supplies the visible label instead. */
  ariaLabel?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: number] }>()

// Clamps rather than refusing, so a value off-step lands on the bound instead
// of stopping a step short of it — the arithmetic useTerminalFont does.
function nudge(delta: 1 | -1): void {
  const stepped = props.next ? props.next(props.modelValue, delta) : props.modelValue + delta * (props.step ?? 1)
  const next = Math.min(props.max, Math.max(props.min, stepped))
  if (next !== props.modelValue) emit('update:modelValue', next)
}
</script>

<template>
  <FormField :label="label" :hint="hint" :testid="testid">
    <div
      class="flex items-center gap-1 rounded-lg border border-row bg-app p-1"
      role="group"
      :aria-label="props.ariaLabel ?? props.label"
    >
      <button
        type="button"
        class="cursor-pointer rounded-md px-2.5 py-1.5 text-small text-text-3 transition-colors hover:text-text-2 disabled:cursor-default disabled:text-text-4/50"
        :disabled="modelValue <= min"
        aria-label="Decrease"
        :data-testid="testid ? `${testid}-decrease` : undefined"
        @click="nudge(-1)"
      >
        −
      </button>
      <span
        class="min-w-[4.5rem] text-center text-small text-text"
        aria-live="polite"
        :data-testid="testid ? `${testid}-value` : undefined"
        >{{ display }}</span
      >
      <button
        type="button"
        class="cursor-pointer rounded-md px-2.5 py-1.5 text-small text-text-3 transition-colors hover:text-text-2 disabled:cursor-default disabled:text-text-4/50"
        :disabled="modelValue >= max"
        aria-label="Increase"
        :data-testid="testid ? `${testid}-increase` : undefined"
        @click="nudge(1)"
      >
        +
      </button>
    </div>
  </FormField>
</template>
