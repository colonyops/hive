<script setup lang="ts">
// An image upload/picker: same "hidden input + trigger button" mechanics as
// SettingsFileField, plus a thumbnail preview rendered from an object URL.
// UI shell only — nothing uploads anywhere.
import { onUnmounted, ref, watch } from 'vue'
import IconImagePlus from '~icons/lucide/image-plus'
import IconX from '~icons/lucide/x'
import SettingsField from './SettingsField.vue'

const props = defineProps<{
  label?: string
  modelValue: File | null
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: File | null] }>()

const inputRef = ref<HTMLInputElement | null>(null)
const previewUrl = ref<string | null>(null)

function revokePreview() {
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = null
}

watch(() => props.modelValue, (file) => {
  revokePreview()
  if (file) previewUrl.value = URL.createObjectURL(file)
}, { immediate: true })

onUnmounted(revokePreview)

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
    <div class="flex items-center gap-3">
      <div
        class="flex size-14 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-strong bg-app text-text-4"
        :data-testid="testid ? `${testid}-preview` : undefined"
      >
        <img v-if="previewUrl" :src="previewUrl" alt="" class="size-full object-cover">
        <IconImagePlus v-else class="size-5" />
      </div>
      <div class="flex min-w-0 flex-1 flex-col gap-1.5">
        <button
          type="button"
          class="flex w-fit cursor-pointer items-center gap-1.5 rounded-lg border border-strong bg-app px-3 py-2 text-[12.5px] text-text-2 hover:border-card hover:text-text"
          :data-testid="testid"
          @click="openPicker"
        ><IconImagePlus class="size-3.5" />Upload image…</button>
        <div v-if="modelValue" class="flex items-center gap-1.5">
          <span class="min-w-0 truncate text-[11.5px] text-text-3" :data-testid="testid ? `${testid}-name` : undefined">{{ modelValue.name }}</span>
          <button
            type="button"
            class="shrink-0 cursor-pointer text-text-3 hover:text-severity-error"
            aria-label="Remove image"
            :data-testid="testid ? `${testid}-clear` : undefined"
            @click="clear"
          ><IconX class="size-3" /></button>
        </div>
      </div>
    </div>
    <input
      ref="inputRef"
      type="file"
      accept="image/*"
      class="hidden"
      :data-testid="testid ? `${testid}-input` : undefined"
      @change="onChange"
    >
  </SettingsField>
</template>
