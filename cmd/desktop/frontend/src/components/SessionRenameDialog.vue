<script setup lang="ts">
// Rename dialog for a hive session. Renaming re-slugs, and the slug is the tmux
// session name, so the hint says what a rename does to an open terminal — the
// app renames the tmux session alongside it (ADR session-rename-keeps-slug-and-tmux-in-step) and re-attaches.
import InlineError from './ui/InlineError.vue'
import { nextTick, onMounted, ref } from 'vue'
import IconPencil from '~icons/lucide/pencil'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import FormField from './ui/FormField.vue'
import TextInput from './ui/TextInput.vue'
import { seedRef } from '../lib/seedRef'

const props = defineProps<{
  name: string
  busy?: boolean
  error?: string | null
}>()
const emit = defineEmits<{ close: []; save: [name: string] }>()

const draft = seedRef(() => props.name)
const inputRef = ref<{ focus: () => void; select: () => void } | null>(null)

function submit(): void {
  if (props.busy) return
  const trimmed = draft.value.trim()
  if (trimmed) emit('save', trimmed)
}

onMounted(async () => {
  await nextTick()
  inputRef.value?.focus()
  inputRef.value?.select()
})
</script>

<template>
  <BaseModal
    title="Rename session"
    :icon="IconPencil"
    :width="440"
    :busy="busy"
    testid="session-rename-dialog"
    @close="emit('close')"
  >
    <div class="px-5 py-4">
      <FormField
        v-slot="{ id }"
        label="Session name"
        hint="Its terminal session is renamed too, so an open terminal reconnects."
        testid="session-rename"
      >
        <TextInput
          :id="id"
          ref="inputRef"
          v-model="draft"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :disabled="busy"
          data-testid="session-rename-input"
          @keydown.enter="submit"
        />
      </FormField>
      <InlineError v-if="error" testid="session-rename-error" class="mt-2.5" :message="error" />
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :busy="busy" data-testid="session-rename-cancel" @click="emit('close')"
        >Cancel</BaseButton
      >
      <BaseButton :busy="busy" :disabled="!draft.trim()" data-testid="session-rename-save" @click="submit"
        >Rename ↵</BaseButton
      >
    </template>
  </BaseModal>
</template>
