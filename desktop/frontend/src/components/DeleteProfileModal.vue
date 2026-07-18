<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import IconTrash2 from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'

const props = defineProps<{ profileName: string; busy: boolean }>()
const emit = defineEmits<{ close: []; confirm: [] }>()

const confirmRef = ref<HTMLButtonElement | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  await nextTick()
  confirmRef.value?.focus()
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 flex items-start justify-center bg-backdrop pt-[24vh]" @click.self="emit('close')">
      <div
        class="w-[420px] overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl"
        role="alertdialog"
        aria-label="Delete profile"
        aria-modal="true"
        data-testid="delete-profile-modal"
      >
        <header class="flex items-center gap-3 border-b border-row px-5 py-4">
          <span class="flex size-7 items-center justify-center rounded-[7px] bg-severity-error-tint text-severity-error"><IconTrash2 class="size-4" /></span>
          <div class="flex-1 text-[15px] font-semibold tracking-[-.01em]">Delete profile</div>
          <button class="cursor-pointer text-text-3 hover:text-text" aria-label="Close" @click="emit('close')"><IconX class="size-4" /></button>
        </header>
        <div class="flex flex-col gap-3 px-5 py-4">
          <p class="text-[13px] leading-relaxed text-text-2">
            Delete <span class="font-semibold text-text">{{ props.profileName }}</span>? Its feeds are removed from
            <span class="font-mono text-text-3">profiles.yaml</span> — sources stay, since other profiles may still
            reference them.
          </p>
        </div>
        <footer class="flex gap-2.5 border-t border-row bg-raised px-5 py-3.5">
          <button
            ref="confirmRef"
            class="flex-1 cursor-pointer rounded-lg bg-severity-error px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110 disabled:cursor-default disabled:opacity-50"
            :disabled="busy"
            data-testid="delete-profile-confirm"
            @click="emit('confirm')"
          >Delete profile</button>
          <button
            class="cursor-pointer rounded-lg border border-card px-4 py-2.5 text-[13.5px] text-text-2 hover:text-text"
            data-testid="delete-profile-cancel"
            @click="emit('close')"
          >Cancel</button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>
