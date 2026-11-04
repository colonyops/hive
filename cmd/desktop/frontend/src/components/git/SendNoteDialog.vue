<script setup lang="ts">
import { ref } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import TextArea from '@hive/ui/components/ui/TextArea.vue'
import IconSend from '~icons/lucide/send'

defineProps<{
  /** How many comments go with the note. */
  count: number
  sendKey: string
}>()
const emit = defineEmits<{ send: [note: string]; close: [] }>()

const note = ref('')

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    event.stopPropagation()
    if (note.value.trim()) emit('send', note.value.trim())
  }
}
</script>

<template>
  <BaseModal title="Send with a note" :icon="IconSend" :width="520" testid="changes-send-note" @close="emit('close')">
    <div class="flex flex-col gap-2 px-5 py-4">
      <p class="text-small text-text-3">
        The agent reads this first, then the {{ count === 1 ? 'comment' : `${count} comments` }}.
      </p>
      <TextArea
        v-model="note"
        :rows="5"
        placeholder="Anything the comments don't say: what to focus on, what to leave alone, when to stop."
        autofocus
        data-testid="changes-send-note-text"
        @keydown="onKeydown"
      />
    </div>
    <template #footer>
      <span class="flex-1" />
      <BaseButton variant="ghost" size="sm" @click="emit('close')">Cancel</BaseButton>
      <BaseButton
        size="sm"
        :disabled="!note.trim()"
        data-testid="changes-send-note-send"
        @click="emit('send', note.trim())"
      >
        Send<span class="ml-1.5 font-mono text-micro opacity-60">{{ sendKey }}</span>
      </BaseButton>
    </template>
  </BaseModal>
</template>
