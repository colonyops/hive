<script setup lang="ts">
// A styled checkbox — the same visual (a bordered square with a check glyph
// when active) FeedEditorSheet uses for its source cards and type filters.
import IconCheck from '~icons/lucide/check'

const props = defineProps<{
  label?: string
  modelValue: boolean
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

function onChange(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).checked)
}
</script>

<template>
  <div>
    <label class="flex cursor-pointer items-center gap-2 text-[13px]" :class="modelValue ? 'text-text' : 'text-text-2'">
      <span
        class="relative flex size-4 shrink-0 items-center justify-center rounded-[5px] border-[1.5px]"
        :class="modelValue ? 'border-accent bg-accent' : 'border-strong bg-transparent'"
      >
        <input
          type="checkbox"
          :checked="modelValue"
          class="absolute inset-0 size-full cursor-pointer opacity-0"
          :data-testid="testid"
          @change="onChange"
        >
        <IconCheck v-if="modelValue" class="size-2.5 text-accent-contrast" :stroke-width="3.2" />
      </span>
      {{ label }}
    </label>
    <p v-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4">{{ hint }}</p>
  </div>
</template>
