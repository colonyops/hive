<script setup lang="ts">
import ConfirmationDialog from './ConfirmationDialog.vue'
const props = defineProps<{ profileName: string; busy: boolean; error?: string | null }>()
const emit = defineEmits<{ close: []; confirm: [] }>()
</script>

<template>
  <ConfirmationDialog
    :title="'Delete profile'"
    :description="`Delete ${props.profileName}? Its flow file and committed feed rows are removed; other profiles keep their own source nodes.`"
    confirm-label="Delete profile"
    :busy="busy"
    :error="error"
    testid="delete-profile-modal"
    confirm-testid="delete-profile-confirm"
    cancel-testid="delete-profile-cancel"
    @cancel="emit('close')"
    @confirm="emit('confirm')"
  />
</template>
