<script setup lang="ts">
// Edit dialog for a sidebar feed folder. Renaming is the job, so the dialog is
// sized to the rename; deleting is the rare exception and stays a quiet footer
// action until it is asked for, at which point it expands into an InlineConfirm
// over a dimmed body. `delete` is emitted only after that confirmation.
import { computed, nextTick, onMounted, ref } from 'vue'
import IconFolder from '~icons/lucide/folder'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import InlineConfirm from './InlineConfirm.vue'
import type { FeedFolder } from '../types/feed'

const props = defineProps<{ folder: FeedFolder }>()
const emit = defineEmits<{ close: []; save: [name: string]; delete: [] }>()

const name = ref(props.folder.name)
const inputRef = ref<HTMLInputElement | null>(null)
const confirming = ref(false)

const count = computed(() => props.folder.feeds.length)
const insideHint = computed(() => (count.value === 0 ? 'No feeds inside' : `${count.value} ${count.value === 1 ? 'feed' : 'feeds'} inside`))
const consequence = computed(() => (count.value === 0
  ? 'The folder is empty, so nothing else changes.'
  : `Its ${count.value} ${count.value === 1 ? 'feed moves' : 'feeds move'} to the top level — nothing is unsubscribed.`))

function submit(): void {
  if (confirming.value) return
  const trimmed = name.value.trim()
  if (trimmed) emit('save', trimmed)
}

// A folder opened straight after creation still carries the placeholder name,
// so the field opens selected and typing replaces it.
onMounted(async () => {
  await nextTick()
  inputRef.value?.focus()
  inputRef.value?.select()
})
</script>

<template>
  <BaseModal
    title="Edit folder"
    :icon="IconFolder"
    :width="460"
    :close-on-backdrop="!confirming"
    :close-on-escape="!confirming"
    testid="folder-edit-modal"
    @close="emit('close')"
  >
    <!-- While a delete is pending the rename recedes: dimmed and inert, so the
         two states can't be misread for each other. -->
    <div class="flex flex-col gap-1.5 px-5 py-4 transition-opacity" :class="{ 'opacity-45': confirming }">
      <label for="folder-edit-name" class="text-xs text-text-3">Folder name</label>
      <input
        id="folder-edit-name"
        ref="inputRef"
        v-model="name"
        type="text"
        :disabled="confirming"
        class="w-full rounded-lg border border-strong bg-raised px-3 py-2.5 text-[13.5px] text-text outline-none focus:border-accent"
        data-testid="folder-edit-name"
        @keydown.enter="submit"
      >
      <span class="text-xs text-text-4" data-testid="folder-edit-inside">{{ insideHint }}</span>
    </div>

    <InlineConfirm
      v-if="confirming"
      title="Delete this folder?"
      :description="consequence"
      confirm-label="Delete"
      cancel-label="Keep"
      testid="folder-delete-confirm"
      @confirm="emit('delete')"
      @cancel="confirming = false"
    />

    <template v-if="!confirming" #footer>
      <button
        type="button"
        class="cursor-pointer text-[12.5px] text-text-3 hover:text-severity-error"
        data-testid="folder-edit-delete"
        @click="confirming = true"
      >Delete folder</button>
      <div class="flex-1" />
      <BaseButton variant="secondary" data-testid="folder-edit-cancel" @click="emit('close')">Cancel</BaseButton>
      <BaseButton :disabled="!name.trim()" data-testid="folder-edit-save" @click="submit">Save ↵</BaseButton>
    </template>
  </BaseModal>
</template>
