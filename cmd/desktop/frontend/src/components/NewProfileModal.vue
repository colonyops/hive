<script setup lang="ts">
import { ref } from 'vue'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import { useAutofocus } from '../composables/useAutofocus'

const props = defineProps<{ busy: boolean; error: string | null }>()
const emit = defineEmits<{ close: []; create: [name: string] }>()

const name = ref('')
const inputRef = ref<HTMLInputElement | null>(null)

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
      <input
        ref="inputRef"
        v-model="name"
        type="text"
        placeholder="Frontend Triage"
        class="w-full rounded-lg border border-strong bg-app px-3.5 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
        data-testid="new-profile-input"
        @keydown.enter="submit"
      >
      <p class="text-xs leading-relaxed text-text-4">
        Saved as a flow in <span class="font-mono text-text-3">flows/</span> with the default feeds — your open PRs,
        the notifications inbox, and cross-repo assignments.
      </p>
      <p v-if="error" class="text-xs text-kind-issue" data-testid="new-profile-error">{{ error }}</p>
    </div>
    <template #footer>
      <BaseButton
        class="flex-1"
        :busy="busy"
        :disabled="!name.trim()"
        data-testid="new-profile-submit"
        @click="submit"
      >Create profile ↵</BaseButton>
      <BaseButton variant="secondary" @click="emit('close')">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
