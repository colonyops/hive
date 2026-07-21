<script setup lang="ts">
import { ref } from 'vue'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import { useAutofocus } from '../composables/useAutofocus'

const props = withDefaults(defineProps<{
  title: string
  description: string
  confirmLabel?: string
  busy?: boolean
  error?: string | null
  testid?: string
  confirmTestid?: string
  cancelTestid?: string
}>(), { confirmLabel: 'Confirm', busy: false, error: null, testid: 'confirmation-dialog', confirmTestid: undefined, cancelTestid: undefined })
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const confirmRef = ref<{ focus: () => void } | null>(null)

function cancel(): void { if (!props.busy) emit('cancel') }

useAutofocus(confirmRef)
</script>

<template>
  <BaseModal
    :title="title"
    :icon="IconAlertTriangle"
    tone="danger"
    aria-role="alertdialog"
    :busy="busy"
    :testid="testid"
    @close="cancel"
  >
    <div class="flex flex-col gap-3 px-5 py-4">
      <p class="text-[13px] leading-relaxed text-text-2">{{ description }}</p>
      <p v-if="error" class="rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" :data-testid="`${testid}-error`">{{ error }}</p>
    </div>
    <template #footer>
      <BaseButton ref="confirmRef" variant="danger" class="flex-1" :busy="busy" :data-testid="confirmTestid ?? `${testid}-confirm`" @click="emit('confirm')">{{ busy ? 'Working…' : confirmLabel }}</BaseButton>
      <BaseButton variant="secondary" :busy="busy" :data-testid="cancelTestid ?? `${testid}-cancel`" @click="cancel">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
