<script setup lang="ts">
import ConfirmationDialog from './ConfirmationDialog.vue'
import type { Confirmation } from '../../composables/useConfirmation'

defineProps<{ confirmation: Confirmation; testid?: string }>()
</script>

<template>
  <ConfirmationDialog
    v-if="confirmation.options.value"
    :title="confirmation.options.value.title"
    :description="confirmation.options.value.description"
    :details="confirmation.options.value.details"
    :confirm-label="confirmation.options.value.confirmLabel"
    :busy="confirmation.busy.value"
    :error="confirmation.error.value"
    :testid="confirmation.options.value.testid ?? testid"
    :confirm-testid="confirmation.options.value.confirmTestid"
    @confirm="confirmation.confirm"
    @cancel="confirmation.cancel"
  />
</template>
