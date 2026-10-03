<script setup lang="ts">
// Rename dialog for an agent chat, mirroring SessionRenameDialog.vue. Unlike
// a hive session, a chat's name is presentation only — the tmux session is
// addressed by record id — so there is no reconnect hint to give.
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
    title="Rename chat"
    :icon="IconPencil"
    :width="440"
    :busy="busy"
    testid="chat-rename-dialog"
    @close="emit('close')"
  >
    <div class="px-5 py-4">
      <FormField v-slot="{ id }" label="Chat name" testid="chat-rename">
        <TextInput
          :id="id"
          ref="inputRef"
          v-model="draft"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :disabled="busy"
          data-testid="chat-rename-input"
          @keydown.enter="submit"
        />
      </FormField>
      <InlineError v-if="error" testid="chat-rename-error" class="mt-2.5" :message="error" />
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :disabled="busy" data-testid="chat-rename-cancel" @click="emit('close')"
        >Cancel</BaseButton
      >
      <BaseButton :busy="busy" :disabled="!draft.trim()" data-testid="chat-rename-save" @click="submit"
        >Rename ↵</BaseButton
      >
    </template>
  </BaseModal>
</template>
