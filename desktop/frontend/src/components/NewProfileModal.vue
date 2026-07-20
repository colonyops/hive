<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import IconX from '~icons/lucide/x'

const props = defineProps<{ busy: boolean; error: string | null }>()
const emit = defineEmits<{ close: []; create: [name: string] }>()

const name = ref('')
const inputRef = ref<HTMLInputElement | null>(null)

function submit() {
  if (props.busy) return
  const trimmed = name.value.trim()
  if (trimmed) emit('create', trimmed)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  await nextTick()
  inputRef.value?.focus()
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
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
          <button
            class="flex-1 cursor-pointer rounded-lg bg-accent px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110 disabled:cursor-default disabled:opacity-50"
            :disabled="busy || !name.trim()"
            data-testid="new-profile-submit"
            @click="submit"
          >Create profile ↵</button>
          <button class="cursor-pointer rounded-lg border border-card px-4 py-2.5 text-[13.5px] text-text-2 hover:text-text" @click="emit('close')">Cancel</button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>
