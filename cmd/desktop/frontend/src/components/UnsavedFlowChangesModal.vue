<script setup lang="ts">
// Navigation guard (hc-sx4k3c7k): shown when the user tries to exit the
// flows canvas or switch the active profile while the active flow has
// un-deployed changes (session.dirty). Mirrors DeleteProfileModal's
// structure/styling, extended with a third action — Deploy writes the
// draft before proceeding, Discard drops it (App.vue calls
// session.discardDraft(), which reloads the flow fresh from disk), Cancel
// aborts the navigation entirely and leaves the draft untouched.
import { ref } from 'vue'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import { useAutofocus } from '../composables/useAutofocus'

const props = defineProps<{ busy: boolean; error?: string | null }>()
const emit = defineEmits<{ close: []; deploy: []; discard: [] }>()

const deployRef = ref<{ focus: () => void } | null>(null)

useAutofocus(deployRef)
</script>

<template>
  <BaseModal
    title="Un-deployed changes"
    :icon="IconTriangleAlert"
    aria-role="alertdialog"
    :busy="busy"
    testid="unsaved-flow-changes-modal"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-3 px-5 py-4">
      <p class="text-[13px] leading-relaxed text-text-2">
        This profile's flow has un-deployed changes. Deploy them now, or discard the draft to continue without them.
      </p>
      <p v-if="error" class="text-xs text-severity-error" data-testid="unsaved-flow-error">{{ error }}</p>
    </div>
    <template #footer>
      <div class="flex w-full flex-col gap-2">
        <div class="flex gap-2.5">
          <BaseButton
            ref="deployRef"
            class="flex-1"
            :busy="busy"
            data-testid="unsaved-flow-deploy"
            @click="emit('deploy')"
          >{{ busy ? 'Deploying…' : 'Deploy' }}</BaseButton>
          <button
            class="flex-1 cursor-pointer rounded-lg border border-severity-error/50 px-4 py-2.5 text-[13.5px] font-semibold text-severity-error hover:bg-severity-error-tint disabled:cursor-default disabled:opacity-50"
            :disabled="busy"
            data-testid="unsaved-flow-discard"
            @click="emit('discard')"
          >Discard changes</button>
        </div>
        <BaseButton
          variant="ghost"
          :busy="busy"
          data-testid="unsaved-flow-cancel"
          @click="emit('close')"
        >Cancel</BaseButton>
      </div>
    </template>
  </BaseModal>
</template>
