<script setup lang="ts">
import InlineError from './ui/InlineError.vue'
import { ref } from 'vue'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import TextInput from './ui/TextInput.vue'
import { useAutofocus } from '../composables/useAutofocus'

const props = defineProps<{ busy: boolean; error: string | null }>()
const emit = defineEmits<{ close: []; create: [name: string] }>()

const name = ref('')
const inputRef = ref<{ focus: () => void } | null>(null)

function submit() {
  if (props.busy) return
  const trimmed = name.value.trim()
  if (trimmed) emit('create', trimmed)
}

useAutofocus(inputRef)
</script>

<template>
  <BaseModal title="New profile" :icon="IconLayoutGrid" testid="new-profile-modal" @close="emit('close')">
    <div class="flex flex-col gap-3 px-5 py-4">
      <TextInput
        ref="inputRef"
        v-model="name"
        placeholder="Frontend Triage"
        data-testid="new-profile-input"
        @keydown.enter="submit"
      />
      <p class="text-xs leading-relaxed text-text-4">
        Saved as a flow in <span class="font-mono text-text-3">flows/</span> with the default feeds — your open PRs, the
        notifications inbox, and cross-repo assignments.
      </p>
      <InlineError v-if="error" testid="new-profile-error" variant="line" :message="error" />
    </div>
    <template #footer>
      <BaseButton class="flex-1" :busy="busy" :disabled="!name.trim()" data-testid="new-profile-submit" @click="submit"
        >Create profile ↵</BaseButton
      >
      <BaseButton variant="secondary" @click="emit('close')">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
