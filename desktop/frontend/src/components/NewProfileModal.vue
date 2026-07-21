<script setup lang="ts">
import { ref } from 'vue'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import IconX from '~icons/lucide/x'
import BaseButton from './BaseButton.vue'
import { useAutofocus } from '../composables/useAutofocus'
import { useEscapeToClose } from '../composables/useEscapeToClose'

const props = defineProps<{ busy: boolean; error: string | null }>()
const emit = defineEmits<{ close: []; create: [name: string] }>()

const name = ref('')
const inputRef = ref<HTMLInputElement | null>(null)

function submit() {
  if (props.busy) return
  const trimmed = name.value.trim()
  if (trimmed) emit('create', trimmed)
}

useEscapeToClose(() => emit('close'))
useAutofocus(inputRef)
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 flex items-start justify-center bg-backdrop pt-[24vh]" @click.self="emit('close')">
      <div
        class="w-[420px] overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl"
        role="dialog"
        aria-label="New profile"
        aria-modal="true"
        data-testid="new-profile-modal"
      >
        <header class="flex items-center gap-3 border-b border-row px-5 py-4">
          <span class="flex size-7 items-center justify-center rounded-[7px] bg-accent-tint text-accent"><IconLayoutGrid class="size-4" /></span>
          <div class="flex-1 text-[15px] font-semibold tracking-[-.01em]">New profile</div>
          <button class="cursor-pointer text-text-3 hover:text-text" aria-label="Close" @click="emit('close')"><IconX class="size-4" /></button>
        </header>
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
        <footer class="flex gap-2.5 border-t border-row bg-raised px-5 py-3.5">
          <BaseButton
            class="flex-1"
            :busy="busy"
            :disabled="!name.trim()"
            data-testid="new-profile-submit"
            @click="submit"
          >Create profile ↵</BaseButton>
          <BaseButton variant="secondary" @click="emit('close')">Cancel</BaseButton>
        </footer>
      </div>
    </div>
  </Teleport>
</template>
