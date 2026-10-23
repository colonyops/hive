<script setup lang="ts">
import { ref } from 'vue'

// Always a string, even for type="number": native v-model would cast a
// number input's value, and callers already parse it themselves.
const model = defineModel<string>({ default: '' })

withDefaults(
  defineProps<{
    /** A text-like type: `text`, `url`, `password`, `number`, `time`. */
    type?: string
    monospace?: boolean
    /** `md` for forms and dialogs; `sm` for drawers and rows beside a small button. */
    size?: 'sm' | 'md'
    invalid?: boolean
  }>(),
  { type: 'text', size: 'md' },
)

const input = ref<HTMLInputElement | null>(null)

function onInput(event: Event): void {
  model.value = (event.target as HTMLInputElement).value
}

defineExpose({
  focus: () => input.value?.focus(),
  select: () => input.value?.select(),
})
</script>

<template>
  <input
    ref="input"
    :value="model"
    :type="type"
    class="w-full rounded-lg border bg-app text-body text-text outline-none placeholder:text-text-4 focus:border-accent disabled:cursor-not-allowed disabled:opacity-60"
    :class="[
      size === 'sm' ? 'px-[11px] py-[9px]' : 'px-3 py-2.5',
      invalid ? 'border-severity-error' : 'border-strong',
      { 'font-mono': monospace },
    ]"
    :aria-invalid="invalid || undefined"
    @input="onInput"
  />
</template>
