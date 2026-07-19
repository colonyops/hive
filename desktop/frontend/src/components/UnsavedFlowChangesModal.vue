<script setup lang="ts">
// Navigation guard (hc-sx4k3c7k): shown when the user tries to exit the
// flows canvas or switch the active profile while the active flow has
// un-deployed changes (session.dirty). Mirrors DeleteProfileModal's
// structure/styling, extended with a third action — Deploy writes the
// draft before proceeding, Discard drops it (App.vue calls
// session.discardDraft(), which reloads the flow fresh from disk), Cancel
// aborts the navigation entirely and leaves the draft untouched.
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconX from '~icons/lucide/x'

const props = defineProps<{ busy: boolean; error?: string | null }>()
const emit = defineEmits<{ close: []; deploy: []; discard: [] }>()

const deployRef = ref<HTMLButtonElement | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && !props.busy) emit('close')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  await nextTick()
  deployRef.value?.focus()
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 flex items-start justify-center bg-backdrop pt-[24vh]" @click.self="!busy && emit('close')">
      <div
        class="w-[420px] overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl"
        role="alertdialog"
        aria-label="Un-deployed changes"
        aria-modal="true"
        data-testid="unsaved-flow-changes-modal"
      >
        <header class="flex items-center gap-3 border-b border-row px-5 py-4">
          <span class="flex size-7 items-center justify-center rounded-[7px] bg-accent-tint text-accent"><IconTriangleAlert class="size-4" /></span>
          <div class="flex-1 text-[15px] font-semibold tracking-[-.01em]">Un-deployed changes</div>
          <button class="cursor-pointer text-text-3 hover:text-text disabled:opacity-50" aria-label="Close" :disabled="busy" @click="emit('close')"><IconX class="size-4" /></button>
        </header>
        <div class="flex flex-col gap-3 px-5 py-4">
          <p class="text-[13px] leading-relaxed text-text-2">
            This profile's flow has un-deployed changes. Deploy them now, or discard the draft to continue without them.
          </p>
          <p v-if="error" class="text-xs text-severity-error" data-testid="unsaved-flow-error">{{ error }}</p>
        </div>
        <footer class="flex flex-col gap-2 border-t border-row bg-raised px-5 py-3.5">
          <div class="flex gap-2.5">
            <button
              ref="deployRef"
              class="flex-1 cursor-pointer rounded-lg bg-accent px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110 disabled:cursor-default disabled:opacity-50"
              :disabled="busy"
              data-testid="unsaved-flow-deploy"
              @click="emit('deploy')"
            >{{ busy ? 'Deploying…' : 'Deploy' }}</button>
            <button
              class="flex-1 cursor-pointer rounded-lg border border-severity-error/50 px-4 py-2.5 text-[13.5px] font-semibold text-severity-error hover:bg-severity-error-tint disabled:cursor-default disabled:opacity-50"
              :disabled="busy"
              data-testid="unsaved-flow-discard"
              @click="emit('discard')"
            >Discard changes</button>
          </div>
          <button
            class="cursor-pointer rounded-lg px-4 py-2 text-center text-[12.5px] text-text-2 hover:text-text disabled:cursor-default disabled:opacity-50"
            :disabled="busy"
            data-testid="unsaved-flow-cancel"
            @click="emit('close')"
          >Cancel</button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>
