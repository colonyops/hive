<script setup lang="ts">
import { ref } from 'vue'

const model = defineModel<string>({ default: '' })

withDefaults(
  defineProps<{
    rows?: number
    monospace?: boolean
    /** Matches TextInput's sizes. */
    size?: 'sm' | 'md'
    invalid?: boolean
  }>(),
  { rows: 3, size: 'md' },
)

const textarea = ref<HTMLTextAreaElement | null>(null)

function onInput(event: Event): void {
  model.value = (event.target as HTMLTextAreaElement).value
}

defineExpose({
  focus: () => textarea.value?.focus(),
  select: () => textarea.value?.select(),
})
</script>

<template>
  <textarea
    ref="textarea"
    :value="model"
    :rows="rows"
    class="w-full resize-y rounded-lg border bg-app text-text outline-none placeholder:text-text-4 focus:border-accent disabled:cursor-not-allowed disabled:opacity-60"
    :class="[
      size === 'sm' ? 'px-[11px] py-[9px] text-[13px]' : 'px-3 py-2.5 text-[13.5px]',
      invalid ? 'border-severity-error' : 'border-strong',
      { 'font-mono': monospace },
    ]"
    :aria-invalid="invalid || undefined"
    @input="onInput"
  />
</template>
