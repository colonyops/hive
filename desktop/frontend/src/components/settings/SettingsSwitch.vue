<script setup lang="ts">
// A boolean switch row — label + hint on the left, a pill switch on the
// right. The pill visual mirrors NodeEditorDrawer.vue's header "Enabled"
// toggle (role="switch" + aria-checked + a sliding dot), the codebase's
// existing precedent for a switch (as opposed to ToggleField.vue's checkbox
// square, used for form checkboxes elsewhere).
const props = defineProps<{
  label: string
  modelValue: boolean
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

function toggle() {
  emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <div class="flex items-center justify-between gap-4 rounded-lg border border-strong bg-app px-3.5 py-3">
    <div class="min-w-0">
      <div class="text-[13px] text-text">{{ label }}</div>
      <p v-if="hint" class="mt-0.5 text-xs leading-relaxed text-text-4">{{ hint }}</p>
    </div>
    <button
      type="button"
      role="switch"
      :aria-checked="modelValue"
      :aria-label="label"
      class="flex shrink-0 cursor-pointer items-center"
      :data-testid="testid"
      @click="toggle"
    >
      <span
        class="relative h-[17px] w-[30px] shrink-0 rounded-full transition-colors"
        :style="{ background: modelValue ? 'var(--color-accent)' : 'var(--color-chip)' }"
      >
        <span
          class="absolute top-[2px] size-[13px] rounded-full bg-white transition-[left]"
          :style="{ left: modelValue ? '15px' : '2px' }"
        />
      </span>
    </button>
  </div>
</template>
