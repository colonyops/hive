<script setup lang="ts">
// Rename dialog for an agent chat, mirroring SessionRenameDialog.vue. Unlike
// a hive session, a chat's name is presentation only — the tmux session is
// addressed by record id — so there is no reconnect hint to give.
import { nextTick, onMounted, ref } from 'vue'
import IconPencil from '~icons/lucide/pencil'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'

const props = defineProps<{
  name: string
  busy?: boolean
  error?: string | null
}>()
const emit = defineEmits<{ close: []; save: [name: string] }>()

const draft = ref(props.name)
const inputRef = ref<HTMLInputElement | null>(null)

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
    <div class="flex flex-col gap-1.5 px-5 py-4">
      <label for="chat-rename-name" class="text-xs text-text-3">Chat name</label>
      <input
        id="chat-rename-name"
        ref="inputRef"
        v-model="draft"
        type="text"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        :disabled="busy"
        class="w-full rounded-lg border border-strong bg-raised px-3 py-2.5 text-[13.5px] text-text outline-none focus:border-accent"
        data-testid="chat-rename-input"
        @keydown.enter="submit"
      >
      <p
        v-if="error"
        class="mt-1 rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error"
        data-testid="chat-rename-error"
      >{{ error }}</p>
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :busy="busy" data-testid="chat-rename-cancel" @click="emit('close')">Cancel</BaseButton>
      <BaseButton :busy="busy" :disabled="!draft.trim()" data-testid="chat-rename-save" @click="submit">Rename ↵</BaseButton>
    </template>
  </BaseModal>
</template>
