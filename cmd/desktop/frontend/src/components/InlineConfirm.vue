<script setup lang="ts">
// A destructive confirmation that expands in place rather than stacking a
// second dialog on top of the first: a rose-washed strip carrying the
// question, what actually happens, and the two answers.
//
// Use this instead of ConfirmationDialog when the thing being confirmed is
// already inside a surface the user opened deliberately — a modal, a card, a
// settings row. Stacked dialogs double the backdrop, and both would answer the
// same Escape press. This renders its own top border and fills the width it is
// given, so it reads as the bottom edge of its host.
//
// `busy`/`error` mirror ConfirmationDialog's contract, so a host with an async
// action can drive this straight from useConfirmation.
import { ref } from 'vue'
import BaseButton from './BaseButton.vue'
import { useAutofocus } from '../composables/useAutofocus'
import { useEscapeToClose } from '../composables/useEscapeToClose'

const props = withDefaults(defineProps<{
  title: string
  description?: string
  confirmLabel?: string
  cancelLabel?: string
  busy?: boolean
  error?: string | null
  testid?: string
}>(), {
  description: '',
  confirmLabel: 'Delete',
  cancelLabel: 'Keep',
  busy: false,
  error: null,
  testid: 'inline-confirm',
})

const emit = defineEmits<{ confirm: []; cancel: [] }>()
const cancelRef = ref<{ focus: () => void } | null>(null)

function cancel(): void {
  if (!props.busy) emit('cancel')
}

// Focus lands on the safe answer, not the destructive one. The strip typically
// opens inside a form whose own Enter binding must not fire underneath it, so
// focus has to move — but it should move somewhere a stray Enter is harmless.
useAutofocus(cancelRef)
useEscapeToClose(cancel)
</script>

<template>
  <div
    class="flex items-center gap-2.5 border-t border-severity-error-border bg-severity-error-tint px-5 py-3"
    role="alertdialog"
    :aria-label="title"
    :data-testid="testid"
  >
    <div class="min-w-0 flex-1">
      <div class="text-[12.5px] font-semibold text-severity-error">{{ title }}</div>
      <p
        v-if="description"
        class="mt-0.5 text-[11.5px] leading-snug text-text-2"
        :data-testid="`${testid}-description`"
      >{{ description }}</p>
      <p
        v-if="error"
        class="mt-1 text-[11.5px] font-medium text-severity-error"
        :data-testid="`${testid}-error`"
      >{{ error }}</p>
    </div>
    <BaseButton
      ref="cancelRef"
      variant="secondary"
      size="sm"
      class="shrink-0"
      :busy="busy"
      :data-testid="`${testid}-cancel`"
      @click="cancel"
    >{{ cancelLabel }}</BaseButton>
    <BaseButton
      variant="danger-outline"
      size="sm"
      class="shrink-0"
      :busy="busy"
      :data-testid="`${testid}-confirm`"
      @click="emit('confirm')"
    >{{ busy ? 'Working…' : confirmLabel }}</BaseButton>
  </div>
</template>
