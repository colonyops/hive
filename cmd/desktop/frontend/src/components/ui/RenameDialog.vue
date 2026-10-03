<script setup lang="ts">
import { nextTick, onMounted, ref, type Component } from 'vue'
import IconPencil from '~icons/lucide/pencil'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import FormField from './FormField.vue'
import TextInput from './TextInput.vue'
import { seedRef } from '../../lib/seedRef'

const props = withDefaults(
  defineProps<{
    title: string
    label: string
    name?: string
    hint?: string
    icon?: Component
    confirmLabel?: string
    busy?: boolean
    error?: string | null
    /** Dims and disables the form, hides the footer, and keeps the dialog open: the default slot holds a confirm. */
    locked?: boolean
    testid: string
    /** Older ids that predate the `${testid}-*` scheme. */
    testids?: { modal?: string; input?: string; save?: string }
  }>(),
  { name: '', icon: () => IconPencil, confirmLabel: 'Rename', testids: () => ({}) },
)
const emit = defineEmits<{ close: []; save: [name: string] }>()

const draft = seedRef(() => props.name)
const inputRef = ref<{ focus: () => void; select: () => void } | null>(null)

function submit(): void {
  if (props.busy || props.locked) return
  const trimmed = draft.value.trim()
  if (trimmed) emit('save', trimmed)
}

// Selected, so typing replaces a placeholder name such as a new folder's.
onMounted(async () => {
  await nextTick()
  inputRef.value?.focus()
  inputRef.value?.select()
})
</script>

<template>
  <BaseModal
    :title="title"
    :icon="icon"
    :width="440"
    :busy="busy"
    :close-on-backdrop="!locked"
    :close-on-escape="!locked"
    :testid="testids.modal ?? `${testid}-dialog`"
    @close="emit('close')"
  >
    <FormField
      v-slot="{ id }"
      :label="label"
      :hint="hint"
      :error="error"
      :testid="testid"
      class="px-5 py-4 transition-opacity"
      :class="{ 'opacity-45': locked }"
    >
      <TextInput
        :id="id"
        ref="inputRef"
        v-model="draft"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        :disabled="busy || locked"
        :data-testid="testids.input ?? `${testid}-input`"
        @keydown.enter="submit"
      />
    </FormField>
    <slot />
    <template v-if="!locked" #footer>
      <slot name="footer-start" />
      <div class="flex-1" />
      <BaseButton variant="secondary" :disabled="busy" :data-testid="`${testid}-cancel`" @click="emit('close')"
        >Cancel</BaseButton
      >
      <BaseButton :busy="busy" :disabled="!draft.trim()" :data-testid="testids.save ?? `${testid}-save`" @click="submit"
        >{{ confirmLabel }} ↵</BaseButton
      >
    </template>
  </BaseModal>
</template>
