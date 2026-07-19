<script setup lang="ts">
// A file selector: a styled button that opens the native file dialog (a
// hidden <input type="file">, the standard web pattern since Wails renders
// this as a normal browser view) and shows the chosen file's name. Kept as
// local component state (the file itself, `modelValue: File | null`) — this
// is a UI shell, nothing uploads or persists anywhere.
import { ref } from 'vue'
import IconFile from '~icons/lucide/file'
import IconX from '~icons/lucide/x'
import SettingsField from './SettingsField.vue'

const props = defineProps<{
  label?: string
  modelValue: File | null
  accept?: string
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: File | null] }>()

const inputRef = ref<HTMLInputElement | null>(null)

function openPicker() {
  inputRef.value?.click()
}

function onChange(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0] ?? null
  emit('update:modelValue', file)
}

function clear() {
  emit('update:modelValue', null)
  if (inputRef.value) inputRef.value.value = ''
}
</script>

<template>
  <SettingsField :label="label" :hint="hint" :testid="testid">
    <div class="flex items-center gap-2.5">
      <button
        type="button"
        class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-lg border border-strong bg-app px-3 py-2 text-[12.5px] text-text-2 hover:border-card hover:text-text"
        :data-testid="testid"
        @click="openPicker"
      ><IconFile class="size-3.5" />Choose file…</button>
      <span class="min-w-0 flex-1 truncate text-[12.5px] text-text-3" :data-testid="testid ? `${testid}-name` : undefined">
        {{ modelValue?.name ?? 'No file selected' }}
      </span>
      <button
        v-if="modelValue"
        type="button"
        class="shrink-0 cursor-pointer text-text-3 hover:text-severity-error"
        aria-label="Remove selected file"
        :data-testid="testid ? `${testid}-clear` : undefined"
        @click="clear"
      ><IconX class="size-3.5" /></button>
    </div>
    <input
      ref="inputRef"
      type="file"
      :accept="accept"
      class="hidden"
      :data-testid="testid ? `${testid}-input` : undefined"
      @change="onChange"
    >
  </SettingsField>
</template>
