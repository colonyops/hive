<script setup lang="ts">
import { ref } from 'vue'
import IconEye from '~icons/lucide/eye'
import IconEyeOff from '~icons/lucide/eye-off'
import SettingsField from './SettingsField.vue'

const props = defineProps<{
  label?: string
  modelValue: string
  placeholder?: string
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const revealed = ref(false)

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).value)
}
</script>

<template>
  <SettingsField :label="label" :hint="hint" :testid="testid">
    <div class="relative">
      <input
        :id="testid"
        :type="revealed ? 'text' : 'password'"
        :value="modelValue"
        :placeholder="placeholder"
        autocomplete="off"
        class="w-full rounded-lg border border-strong bg-app py-2.5 pl-3.5 pr-10 font-mono text-[13px] text-text outline-none placeholder:font-sans placeholder:text-text-4 focus:border-accent"
        :data-testid="testid"
        @input="onInput"
      >
      <button
        type="button"
        class="absolute inset-y-0 right-0 flex w-9 cursor-pointer items-center justify-center text-text-3 hover:text-text"
        :aria-label="revealed ? 'Hide secret' : 'Show secret'"
        :aria-pressed="revealed"
        :data-testid="testid ? `${testid}-reveal` : undefined"
        @click="revealed = !revealed"
      >
        <component :is="revealed ? IconEyeOff : IconEye" class="size-3.5" />
      </button>
    </div>
  </SettingsField>
</template>
